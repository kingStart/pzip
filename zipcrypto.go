package zip

import (
	"hash/crc32"
	"io"
)

type ZipCrypto struct {
	password []byte
	Keys     [3]uint32
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
	z.Keys[0] = crc32update(z.Keys[0], byteValue)
	z.Keys[1] += z.Keys[0] & 0xff
	z.Keys[1] = z.Keys[1]*134775813 + 1
	z.Keys[2] = crc32update(z.Keys[2], byte(z.Keys[1]>>24))
}

func (z *ZipCrypto) magicByte() byte {
	var t uint32 = z.Keys[2] | 2
	return byte((t * (t ^ 1)) >> 8)
}

// encryptTo encrypts src into dst in-place (dst and src may overlap or be the
// same slice). For encryption, updateKeys uses the plaintext byte.
func (z *ZipCrypto) encryptTo(dst, src []byte) {
	for i, v := range src {
		dst[i] = v ^ z.magicByte()
		z.updateKeys(v)
	}
}

// decryptInPlace decrypts data in-place, returning the same slice.
func (z *ZipCrypto) decryptInPlace(data []byte) []byte {
	for i, c := range data {
		v := c ^ z.magicByte()
		z.updateKeys(v)
		data[i] = v
	}
	return data
}

// Encrypt returns a new encrypted copy. Kept for backward compatibility.
func (z *ZipCrypto) Encrypt(data []byte) []byte {
	out := make([]byte, len(data))
	z.encryptTo(out, data)
	return out
}

// Decrypt returns a new decrypted copy. Kept for backward compatibility.
func (z *ZipCrypto) Decrypt(chiper []byte) []byte {
	out := make([]byte, len(chiper))
	copy(out, chiper)
	return z.decryptInPlace(out)
}

func crc32update(pCrc32 uint32, bval byte) uint32 {
	return crc32.IEEETable[(pCrc32^uint32(bval))&0xff] ^ (pCrc32 >> 8)
}

// zipCryptoReader implements streaming decryption for ZipCrypto.
type zipCryptoReader struct {
	r          io.Reader
	z          *ZipCrypto
	headerRead bool
	checkByte  byte // 0 means no verification
	verify     bool
}

func (zcr *zipCryptoReader) Read(p []byte) (int, error) {
	if !zcr.headerRead {
		var header [12]byte
		if _, err := io.ReadFull(zcr.r, header[:]); err != nil {
			return 0, err
		}
		zcr.z.decryptInPlace(header[:])
		if zcr.verify && header[11] != zcr.checkByte {
			return 0, ErrPassword
		}
		zcr.headerRead = true
	}

	n, err := zcr.r.Read(p)
	if n > 0 {
		zcr.z.decryptInPlace(p[:n])
	}
	return n, err
}

// ZipCryptoDecryptor creates a streaming decryptor with password verification.
func ZipCryptoDecryptor(r io.Reader, password []byte, checkByte byte) (io.Reader, error) {
	z := NewZipCrypto(password)
	return &zipCryptoReader{
		r:         r,
		z:         z,
		checkByte: checkByte,
		verify:    true,
	}, nil
}

// ZipCryptoDecryptorStream creates a streaming decryptor without password
// verification. Use ZipCryptoDecryptor when check byte is available.
func ZipCryptoDecryptorStream(r io.Reader, password []byte) (io.Reader, error) {
	z := NewZipCrypto(password)
	return &zipCryptoReader{
		r: r,
		z: z,
	}, nil
}

type zipCryptoWriter struct {
	w     io.Writer
	z     *ZipCrypto
	first bool
	fw    *fileWriter
	buf   []byte // reusable encryption buffer
}

func (z *zipCryptoWriter) Write(p []byte) (n int, err error) {
	if z.first {
		z.first = false
		var header [12]byte
		copy(header[:], []byte{0xF8, 0x53, 0xCF, 0x05, 0x2D, 0xDD, 0xAD, 0xC8, 0x66, 0x3F, 0x8C, 0xAC})
		z.z.encryptTo(header[:], header[:])

		crc := z.fw.ModifiedTime
		header[10] = byte(crc)
		header[11] = byte(crc >> 8)

		z.z.init()
		z.z.encryptTo(header[:], header[:])
		if _, err = z.w.Write(header[:]); err != nil {
			return 0, err
		}
	}
	// Grow reusable buffer if needed
	if cap(z.buf) < len(p) {
		z.buf = make([]byte, len(p))
	}
	z.buf = z.buf[:len(p)]
	z.z.encryptTo(z.buf, p)
	nn, err := z.w.Write(z.buf)
	n = nn
	return
}

func ZipCryptoEncryptor(i io.Writer, pass passwordFn, fw *fileWriter) (io.Writer, error) {
	z := NewZipCrypto(pass())
	zc := &zipCryptoWriter{w: i, z: z, first: true, fw: fw}
	return zc, nil
}
