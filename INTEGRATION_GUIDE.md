# pzip v1.1.0 接入集成指南

## 升级方式

### Go Module 依赖更新

```bash
# 在项目目录下更新 pzip 到最新版
go get github.com/kingStart/pzip@latest
go mod tidy
```

### 验证

```bash
go build ./...
go test ./...
```

## 迁移须知

### 零修改即可升级的场景

以下使用方式**无需任何代码修改**即可享受全部修复和新特性：

```go
// 标准读取流程 — 完全兼容
r, err := zip.OpenReader("archive.zip")
for _, f := range r.File {
    rc, err := f.Open()     // 自动流式解密 + 密码验证
    // ...
}

// OpenStream 也完全兼容（现在是 Open 的别名）
rc, err := f.OpenStream()
```

### 需要适配的场景

#### 1. 密码错误处理优化（推荐）

**旧行为**：密码错误 → `flate: corrupt input before offset N`

**新行为**：密码错误 → `zip: invalid password`（`zip.ErrPassword`）

如果你的代码依赖错误字符串判断密码错误，建议改用类型判断：

```go
// 旧代码（仍然有效但不精确）
if strings.Contains(err.Error(), "flate") {
    // 可能是密码错误
}

// 新代码（推荐）
if errors.Is(err, zip.ErrPassword) {
    // 明确是密码错误
}
```

**`gov-doclab` 中的 `zip_util.go` 适配示例**：

```go
// 当前代码中的错误判断：
if errMsg := err.Error(); strings.Contains(errMsg, "flate:") || strings.Contains(errMsg, "corrupt input") {
    return errors.Join(err, ErrZipPasswordWrong)
}

// 建议改为：
if errors.Is(err, zip.ErrPassword) {
    return errors.Join(err, ErrZipPasswordWrong)
}
```

#### 2. 直接调用 `ZipCryptoDecryptor` 的代码

旧签名：
```go
func ZipCryptoDecryptor(r *io.SectionReader, password []byte) (*io.SectionReader, error)
```

新签名：
```go
func ZipCryptoDecryptor(r io.Reader, password []byte, checkByte byte) (io.Reader, error)
```

变化点：
- 参数 `r` 类型从 `*io.SectionReader` → `io.Reader`
- 新增 `checkByte` 参数（用于密码验证）
- 返回类型从 `*io.SectionReader` → `io.Reader`

如果不方便提供 check byte，可使用无验证版本：
```go
r, err := zip.ZipCryptoDecryptorStream(reader, password)
```

#### 3. `ErrInsecurePath` 处理

新版 `NewReader` / `OpenReader` 会在读取到包含路径穿越的文件名时返回 `ErrInsecurePath`：

```go
r, err := zip.OpenReader("malicious.zip")
if errors.Is(err, zip.ErrInsecurePath) {
    // ZIP 中包含 "../" 或绝对路径的文件名
    // 根据业务需求决定是否拒绝
}
```

## 新特性使用示例

### SetComment — ZIP 全局注释

```go
w := zip.NewWriter(file)
w.SetComment("Created by pzip v1.1.0")
// ... 添加文件 ...
w.Close()
```

### OpenRaw + Copy — 无损复制条目

在 ZIP 间高效复制文件（不解压/不重压缩）：

```go
src, _ := zip.OpenReader("source.zip")
defer src.Close()

out, _ := os.Create("modified.zip")
dst := zip.NewWriter(out)

for _, f := range src.File {
    if f.Name == "skip_this.txt" {
        continue // 跳过不需要的条目
    }
    dst.Copy(f) // 无损复制
}

// 可以追加新文件
w, _ := dst.Create("new_file.txt")
w.Write([]byte("new content"))

dst.Close()
out.Close()
```

### Extended Timestamp — 精确时间戳

读取时自动解析：

```go
r, _ := zip.OpenReader("archive.zip")
for _, f := range r.File {
    fmt.Println(f.Modified) // time.Time，秒精度
}
```

写入时设置 `Modified` 即自动写入扩展时间戳：

```go
fh := &zip.FileHeader{
    Name:     "file.txt",
    Method:   zip.Deflate,
    Modified: time.Now(), // 设置此字段即可
}
w, _ := zw.CreateHeader(fh)
w.Write(data)
```

### baseOffset — 嵌入式 ZIP

自动支持，无需额外代码。以下场景现在可正确读取：

```go
// 读取嵌入在可执行文件中的 ZIP 数据
r, err := zip.NewReader(readerAt, fileSize)
// baseOffset 自动计算，偏移透明处理
```

## 测试验证清单

升级后建议验证以下场景：

- [ ] 正确密码读取加密 ZIP（ZipCrypto）
- [ ] 错误密码读取加密 ZIP → 返回 `ErrPassword`
- [ ] 正确密码读取加密 ZIP（WinZip AES）
- [ ] 读写大文件（>4GB，ZIP64）
- [ ] 读取标准工具（7-Zip、WinRAR）创建的 ZIP
- [ ] 写入的 ZIP 可被标准工具正确读取
- [ ] `Modified` 时间戳在读写后保持一致
