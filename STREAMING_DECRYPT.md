# 流式解密解压缩使用说明

## 背景

原有的 `Open()` 方法在处理 ZipCrypto 加密的文件时，会将整个压缩文件内容加载到内存中再进行解密，导致处理大文件时内存占用过高。例如，解压 50GB 的加密文件可能需要 100GB 内存。

新增的 `OpenStream()` 方法采用流式解密方式，数据在读取过程中实时解密，不需要预先加载全部数据到内存，从而显著降低内存占用。

## 新增 API

### 1. File.OpenStream()

流式解密入口方法，用法与 `Open()` 完全一致，但对 ZipCrypto 加密文件使用流式解密。

```go
func (f *File) OpenStream() (rc io.ReadCloser, err error)
```

**特点：**
- 对于 ZipCrypto 加密文件：使用流式解密，内存占用恒定
- 对于 AES 加密文件：与 `Open()` 行为一致（AES 本身已是流式解密）
- 对于非加密文件：与 `Open()` 行为一致

### 2. ZipCryptoDecryptorStream()

底层流式解密函数，返回一个 `io.Reader`，数据在读取时实时解密。

```go
func ZipCryptoDecryptorStream(r io.Reader, password []byte) (io.Reader, error)
```

## 使用示例

### 示例 1：基本用法（推荐）

```go
package main

import (
    "io"
    "os"
    
    zip "github.com/pzip/pzip"
)

func main() {
    // 打开加密的 zip 文件
    r, err := zip.OpenReader("encrypted.zip")
    if err != nil {
        panic(err)
    }
    defer r.Close()

    for _, f := range r.File {
        // 设置密码
        f.SetPassword("123456")
        
        // 使用流式解密打开文件（推荐用于大文件）
        rc, err := f.OpenStream()
        if err != nil {
            panic(err)
        }
        
        // 创建输出文件
        outFile, err := os.Create(f.Name)
        if err != nil {
            rc.Close()
            panic(err)
        }
        
        // 流式复制，内存占用恒定
        _, err = io.Copy(outFile, rc)
        
        outFile.Close()
        rc.Close()
        
        if err != nil {
            panic(err)
        }
    }
}
```

### 示例 2：分块读取（更精细的内存控制）

```go
package main

import (
    "os"
    
    zip "github.com/pzip/pzip"
)

func main() {
    r, err := zip.OpenReader("large-encrypted.zip")
    if err != nil {
        panic(err)
    }
    defer r.Close()

    for _, f := range r.File {
        f.SetPassword("123456")
        
        rc, err := f.OpenStream()
        if err != nil {
            panic(err)
        }
        
        outFile, _ := os.Create(f.Name)
        
        // 使用固定大小的缓冲区，精确控制内存使用
        buf := make([]byte, 32*1024) // 32KB 缓冲区
        for {
            n, err := rc.Read(buf)
            if n > 0 {
                outFile.Write(buf[:n])
            }
            if err != nil {
                break
            }
        }
        
        outFile.Close()
        rc.Close()
    }
}
```

### 示例 3：直接使用底层 API

```go
package main

import (
    "io"
    "os"
    
    zip "github.com/pzip/pzip"
)

func decryptStream(inputReader io.Reader, password string) (io.Reader, error) {
    return zip.ZipCryptoDecryptorStream(inputReader, []byte(password))
}
```

## 内存对比

| 文件大小 | Open() 内存占用 | OpenStream() 内存占用 |
|---------|----------------|---------------------|
| 1 GB    | ~2 GB          | ~32 KB (缓冲区大小)  |
| 10 GB   | ~20 GB         | ~32 KB (缓冲区大小)  |
| 50 GB   | ~100 GB        | ~32 KB (缓冲区大小)  |

## 兼容性说明

- `OpenStream()` 的返回值类型与 `Open()` 完全一致 (`io.ReadCloser`)
- 解密后的数据内容与 `Open()` 完全一致
- CRC32 校验正常执行
- 支持所有压缩方法（Store、Deflate 等）

## API 选择建议

| 场景 | 推荐方法 |
|-----|---------|
| 小文件（< 100MB） | `Open()` 或 `OpenStream()` |
| 大文件（> 100MB） | `OpenStream()` |
| 内存受限环境 | `OpenStream()` |
| 需要随机访问 | `Open()` (先解密到内存) |

## 注意事项

1. `OpenStream()` 只能顺序读取，不支持 Seek 操作
2. 每次调用 `OpenStream()` 都会从头开始读取
3. 确保在读取完成后调用 `Close()` 释放资源
