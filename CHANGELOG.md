# pzip Changelog

## v1.1.0

### 安全修复

#### ZipCrypto 密码验证（严重）

**问题**：`File.Open()` 对 ZipCrypto 加密文件不做密码验证。错误密码不会立即报错，而是将解密后的垃圾数据传给 DEFLATE 解压器，导致 `flate: corrupt input before offset N` 这种难以诊断的错误。

**修复**：`Open()` 现在通过 12 字节加密头的 check byte（PKWARE APPNOTE 6.1.4）进行前置密码验证。密码错误时立即返回 `ErrPassword`。

**影响范围**：所有使用 `File.Open()` / `File.OpenStream()` 读取 ZipCrypto 加密文件的代码。正确密码不受影响。

#### ZipCrypto 解密 OOM 风险（严重）

**问题**：原 `ZipCryptoDecryptor` 通过 `make([]byte, r.Size())` 将整个压缩数据加载到内存。对于大文件（如 27GB），会导致进程 OOM 被杀。同时忽略 `Read` 返回值，在 I/O 失败时静默使用零填充数据。

**修复**：`Open()` 和 `OpenStream()` 统一使用流式解密，每次 `Read` 仅处理传入的缓冲区大小。内存占用从 O(文件大小) 降至 O(缓冲区大小)。

#### zipCryptoWriter.Write 契约违反（高）

**问题**：`Write` 方法不传播底层写入错误，且返回值不正确：
- 首次写入返回 `n=12`（仅计算头部），不计算实际数据
- 后续写入返回 `n=0`

违反 `io.Writer` 契约，导致上层 `io.Copy` 等工具无法正确检测写入错误。

**修复**：正确传播错误并返回实际写入的数据长度。写出的数据不变，仅修复返回值。

#### WinZip AES Extra 解析 Panic（高）

**问题**：解析 `winzipAesExtraId`（0x9901）时未检查 Extra 数据是否有足够的 7 字节（2+2+1+2）。畸形 ZIP 可导致 `index out of range` panic。

**修复**：添加 `len(eb) < 7` 前置检查，不满足则返回 `ErrFormat`。

#### AES dataLen uint64 下溢（高）

**问题**：`newDecryptionReader` 中 `CompressedSize64 - saltLen - 2 - 10` 在 `CompressedSize64` 过小时发生 uint64 下溢，wraparound 成巨大值，可能导致后续巨大内存分配或读取越界。

**修复**：减法前检查 `CompressedSize64 < overhead`，不满足则返回 `ErrFormat`。

#### 路径穿越检测（中）

**问题**：读取 ZIP 时不检查文件名是否包含路径穿越序列（`../`、绝对路径、Windows 驱动器号）。

**修复**：新增 `ErrInsecurePath` 错误类型。`NewReader` / `OpenReader` 在读取中央目录后检查所有文件名，发现不安全路径时返回该错误。

### 新特性

#### 嵌入式 ZIP 读取（baseOffset）

支持读取嵌入在其他文件中的 ZIP 数据（如 Android APK、Java JAR、自解压归档）。通过计算 `baseOffset` 补偿前缀偏移，使中央目录和本地文件头的偏移能正确对应到文件中的实际位置。

#### Writer.SetComment()

新增 `Writer.SetComment(comment string) error` 方法，支持在 ZIP 尾部写入全局注释（最大 65535 字节）。

#### File.OpenRaw() / Writer.Copy()

- `File.OpenRaw()` 返回文件的原始压缩（可能加密）数据流
- `Writer.Copy(f *File)` 将一个文件条目无损复制到新 ZIP 中

这两个方法配合使用，可以高效地在 ZIP 文件间复制条目而无需解压/重压缩，适用于修改 ZIP 中部分条目时保持其余条目不变。

#### Extended Timestamp（0x5455）

- **读取**：解析 Extra 字段中的扩展时间戳，填充 `FileHeader.Modified`（`time.Time` 类型，秒精度）
- **写入**：当 `FileHeader.Modified` 非零时，自动在 Extra 字段中写入扩展时间戳

相比 MS-DOS 时间的 2 秒精度，扩展时间戳提供 1 秒精度。

### 代码清理

- `os.SEEK_SET` → `io.SeekStart`（废弃 API 替换）
- `OpenStream()` 改为 `Open()` 的直接别名（保持向后兼容）
- 删除过时的 `STREAMING_DECRYPT.md` 文档

### 兼容性

所有修改保持完全向后兼容：

| 场景 | 兼容性 |
|------|--------|
| 新 Reader 读旧 Writer 的 ZIP | ✅ |
| 旧 Reader 读新 Writer 的 ZIP | ✅ |
| 新 Reader 读标准 ZIP 工具的 ZIP | ✅（增强：baseOffset、Extended Timestamp） |
| 新 Writer 写的 ZIP 被标准工具读取 | ✅ |
| `Open()` / `OpenStream()` API | ✅ 签名不变 |
| 已有的 `ZipCryptoDecryptorStream` 公开函数 | ✅ 保留 |

唯一行为变化：`Open()` 在密码错误时会返回 `ErrPassword` 而非 `flate: corrupt input`。这是更精确的错误信息，但调用方如果只检查 `err != nil` 则不受影响。
