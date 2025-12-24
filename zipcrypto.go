package zip

import (
	"io"
	"bytes"
	"hash/crc32"
)

type ZipCrypto struct {
	password []byte
	Keys [3]uint32
}

func NewZipCrypto(passphrase []byte) *ZipCrypto {
	z := &ZipCrypto{}
	z.password = passphrase
	z.init()
	return z
}

func (z *ZipCrypto) init() {
	z.Keys[0] = 0x12345678
	z.Keys[1] = 0x23456789
	z.Keys[2] = 0x34567890

	for i := 0; i < len(z.password); i++ {
		z.updateKeys(z.password[i])
	}
}

func (z *ZipCrypto) updateKeys(byteValue byte) {
	z.Keys[0] = crc32update(z.Keys[0], byteValue);
	z.Keys[1] += z.Keys[0] & 0xff;
	z.Keys[1] = z.Keys[1] * 134775813 + 1;
	z.Keys[2] = crc32update(z.Keys[2], (byte) (z.Keys[1] >> 24));
}

func (z *ZipCrypto) magicByte() byte {
	var t uint32 = z.Keys[2] | 2
	return byte((t * (t ^ 1)) >> 8)
}

func (z *ZipCrypto) Encrypt(data []byte) []byte {
	length := len(data)
	chiper := make([]byte, length)
	for i := 0; i < length; i++ {
		v := data[i]
		chiper[i] = v ^ z.magicByte()
		z.updateKeys(v)
	}
	return chiper
}

func (z *ZipCrypto) Decrypt(chiper []byte) []byte {
	length := len(chiper)
	plain := make([]byte, length)
	for i, c := range chiper {
		v := c ^ z.magicByte();
		z.updateKeys(v)
		plain[i] = v
	}
	return plain
}

func crc32update(pCrc32 uint32, bval byte) uint32 {
	return crc32.IEEETable[(pCrc32 ^ uint32(bval)) & 0xff] ^ (pCrc32 >> 8)
}

// ZipCryptoDecryptor creates a decryptor using buffered mode (loads all data into memory).
// For large files, consider using ZipCryptoDecryptorStream instead.
func ZipCryptoDecryptor(r *io.SectionReader, password []byte) (*io.SectionReader, error) {
	z := NewZipCrypto(password)
	b := make([]byte, r.Size())

	r.Read(b)

	m := z.Decrypt(b)
	return io.NewSectionReader(bytes.NewReader(m), 12, int64(len(m))), nil
}

// zipCryptoReader implements streaming decryption for ZipCrypto.
// It decrypts data on-the-fly without loading the entire file into memory.
type zipCryptoReader struct {
	r           io.Reader
	z           *ZipCrypto
	headerRead  bool
	remaining   int64 // remaining bytes to read (-1 for unknown)
}

// Read implements io.Reader interface for streaming decryption.
func (zcr *zipCryptoReader) Read(p []byte) (int, error) {
	if !zcr.headerRead {
		// Read and decrypt the 12-byte encryption header first
		header := make([]byte, 12)
		if _, err := io.ReadFull(zcr.r, header); err != nil {
			return 0, err
		}
		// Decrypt header (updates keys state)
		zcr.z.Decrypt(header)
		zcr.headerRead = true
	}

	// Read and decrypt data in streaming fashion
	n, err := zcr.r.Read(p)
	if n > 0 {
		// Decrypt in-place
		for i := 0; i < n; i++ {
			v := p[i] ^ zcr.z.magicByte()
			zcr.z.updateKeys(v)
			p[i] = v
		}
	}
	return n, err
}

// ZipCryptoDecryptorStream creates a streaming decryptor that decrypts data on-the-fly.
// This method is memory-efficient for large files as it doesn't load all data into memory.
// Returns an io.Reader that produces decrypted data.
func ZipCryptoDecryptorStream(r io.Reader, password []byte) (io.Reader, error) {
	z := NewZipCrypto(password)
	return &zipCryptoReader{
		r:          r,
		z:          z,
		headerRead: false,
	}, nil
}

// ZipCryptoDecryptorStreamWithSize creates a streaming decryptor with known size.
// This is useful when you need to track the remaining bytes.
func ZipCryptoDecryptorStreamWithSize(r io.Reader, password []byte, size int64) (io.Reader, error) {
	z := NewZipCrypto(password)
	return &zipCryptoReader{
		r:          r,
		z:          z,
		headerRead: false,
		remaining:  size - 12, // subtract header size
	}, nil
}

type zipCryptoWriter struct {
	w     io.Writer
	z     *ZipCrypto
	first bool
	fw    *fileWriter
}

func (z *zipCryptoWriter) Write(p []byte) (n int, err error) {
	err = nil
	if z.first {
		z.first = false
		header := []byte{0xF8, 0x53, 0xCF, 0x05, 0x2D, 0xDD, 0xAD, 0xC8, 0x66, 0x3F, 0x8C, 0xAC}
		header = z.z.Encrypt(header)

		crc := z.fw.ModifiedTime
		header[10] = byte(crc)
		header[11] = byte(crc >> 8)

		z.z.init()
		z.w.Write(z.z.Encrypt(header))
		n += 12
	}
	z.w.Write(z.z.Encrypt(p))
	return
}

func ZipCryptoEncryptor(i io.Writer, pass passwordFn, fw *fileWriter) (io.Writer, error)  {
	z := NewZipCrypto(pass())
	zc := &zipCryptoWriter{i, z, true, fw}
	return zc, nil
}