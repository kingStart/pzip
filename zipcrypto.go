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
		v := c ^ z.magicByte()
		z.updateKeys(v)
		plain[i] = v
	}
	return plain
}

func crc32update(pCrc32 uint32, bval byte) uint32 {
	return crc32.IEEETable[(pCrc32^uint32(bval))&0xff] ^ (pCrc32 >> 8)
}

// zipCryptoReader implements streaming decryption for ZipCrypto.
type zipCryptoReader struct {
	r          io.Reader
	z          *ZipCrypto
	headerRead bool
}

func (zcr *zipCryptoReader) Read(p []byte) (int, error) {
	if !zcr.headerRead {
		header := make([]byte, 12)
		if _, err := io.ReadFull(zcr.r, header); err != nil {
			return 0, err
		}
		zcr.z.Decrypt(header)
		zcr.headerRead = true
	}

	n, err := zcr.r.Read(p)
	if n > 0 {
		for i := 0; i < n; i++ {
			v := p[i] ^ zcr.z.magicByte()
			zcr.z.updateKeys(v)
			p[i] = v
		}
	}
	return n, err
}

// zipCryptoValidatingReader adds password verification via the 12-byte
// encryption header check byte (PKWARE APPNOTE 6.1.4).
type zipCryptoValidatingReader struct {
	r          io.Reader
	z          *ZipCrypto
	headerRead bool
	checkByte  byte // expected high byte of ModifiedTime (data descriptor) or CRC-32
}

func (zcr *zipCryptoValidatingReader) Read(p []byte) (int, error) {
	if !zcr.headerRead {
		header := make([]byte, 12)
		if _, err := io.ReadFull(zcr.r, header); err != nil {
			return 0, err
		}
		decrypted := zcr.z.Decrypt(header)
		if decrypted[11] != zcr.checkByte {
			return 0, ErrPassword
		}
		zcr.headerRead = true
	}

	n, err := zcr.r.Read(p)
	if n > 0 {
		for i := 0; i < n; i++ {
			v := p[i] ^ zcr.z.magicByte()
			zcr.z.updateKeys(v)
			p[i] = v
		}
	}
	return n, err
}

// ZipCryptoDecryptor creates a streaming decryptor with password verification.
// The checkByte should be the high byte of ModifiedTime (when data descriptor
// flag is set) or the high byte of CRC-32.
func ZipCryptoDecryptor(r io.Reader, password []byte, checkByte byte) (io.Reader, error) {
	z := NewZipCrypto(password)
	return &zipCryptoValidatingReader{
		r:         r,
		z:         z,
		checkByte: checkByte,
	}, nil
}

// ZipCryptoDecryptorStream creates a streaming decryptor without password
// verification. Use ZipCryptoDecryptor for new code when check byte is available.
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
}

func (z *zipCryptoWriter) Write(p []byte) (n int, err error) {
	if z.first {
		z.first = false
		header := []byte{0xF8, 0x53, 0xCF, 0x05, 0x2D, 0xDD, 0xAD, 0xC8, 0x66, 0x3F, 0x8C, 0xAC}
		header = z.z.Encrypt(header)

		crc := z.fw.ModifiedTime
		header[10] = byte(crc)
		header[11] = byte(crc >> 8)

		z.z.init()
		if _, err = z.w.Write(z.z.Encrypt(header)); err != nil {
			return 0, err
		}
	}
	nn, err := z.w.Write(z.z.Encrypt(p))
	n = nn
	return
}

func ZipCryptoEncryptor(i io.Writer, pass passwordFn, fw *fileWriter) (io.Writer, error) {
	z := NewZipCrypto(pass())
	zc := &zipCryptoWriter{i, z, true, fw}
	return zc, nil
}
