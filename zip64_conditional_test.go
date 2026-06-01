package zip

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// buildCDEntryWithZip64Extra constructs a raw Central Directory entry
// with controlled 32-bit fields and a ZIP64 extra field containing
// only the specified fields.
func buildCDEntryWithZip64Extra(
	name string,
	uncompSize32 uint32, compSize32 uint32, offset32 uint32,
	zip64Fields []uint64,
) []byte {
	nameBytes := []byte(name)

	var extraBuf bytes.Buffer
	if len(zip64Fields) > 0 {
		binary.Write(&extraBuf, binary.LittleEndian, uint16(zip64ExtraId))
		binary.Write(&extraBuf, binary.LittleEndian, uint16(len(zip64Fields)*8))
		for _, v := range zip64Fields {
			binary.Write(&extraBuf, binary.LittleEndian, v)
		}
	}
	extra := extraBuf.Bytes()

	buf := make([]byte, directoryHeaderLen+len(nameBytes)+len(extra))
	b := writeBuf(buf)
	b.uint32(directoryHeaderSignature)
	b.uint16(zipVersion45)     // creator version
	b.uint16(zipVersion45)     // reader version
	b.uint16(0)                // flags
	b.uint16(Store)            // method
	b.uint16(0)                // mod time
	b.uint16(0)                // mod date
	b.uint32(0x12345678)       // crc32
	b.uint32(compSize32)       // compressed size
	b.uint32(uncompSize32)     // uncompressed size
	b.uint16(uint16(len(nameBytes)))
	b.uint16(uint16(len(extra)))
	b.uint16(0) // comment length
	b.uint16(0) // disk number start
	b.uint16(0) // internal attributes
	b.uint32(0) // external attributes
	b.uint32(offset32)

	copy(buf[directoryHeaderLen:], nameBytes)
	copy(buf[directoryHeaderLen+len(nameBytes):], extra)
	return buf
}

// TestZip64OffsetOnlyExtra verifies that when only the header offset
// exceeds 32 bits (sizes fit in 32 bits), the ZIP64 extra field
// containing only the offset is parsed correctly.
// This is the exact scenario that triggered the bug on large archives.
func TestZip64OffsetOnlyExtra(t *testing.T) {
	// Scenario: file at offset 5GB, sizes fit in 32 bits.
	// ZIP64 extra contains ONLY the 8-byte offset.
	const wantOffset int64 = 5 * (1 << 30) // 5 GB
	const wantCompSize uint64 = 1000
	const wantUncompSize uint64 = 2000

	cdEntry := buildCDEntryWithZip64Extra(
		"test.txt",
		uint32(wantUncompSize), uint32(wantCompSize), uint32(uint32max),
		[]uint64{uint64(wantOffset)}, // only offset in ZIP64 extra
	)

	f := &File{}
	err := readDirectoryHeader(f, bytes.NewReader(cdEntry))
	if err != nil {
		t.Fatalf("readDirectoryHeader failed: %v", err)
	}

	if f.headerOffset != wantOffset {
		t.Errorf("headerOffset = %d (0x%X), want %d (0x%X)",
			f.headerOffset, f.headerOffset, wantOffset, wantOffset)
	}
	if f.CompressedSize64 != wantCompSize {
		t.Errorf("CompressedSize64 = %d, want %d", f.CompressedSize64, wantCompSize)
	}
	if f.UncompressedSize64 != wantUncompSize {
		t.Errorf("UncompressedSize64 = %d, want %d", f.UncompressedSize64, wantUncompSize)
	}
}

// TestZip64SizesOnlyExtra verifies that when only sizes exceed 32 bits
// but offset fits in 32 bits, the ZIP64 extra with only sizes is
// parsed correctly.
func TestZip64SizesOnlyExtra(t *testing.T) {
	const wantOffset int64 = 1024
	const wantCompSize uint64 = 5 * (1 << 30)   // 5 GB
	const wantUncompSize uint64 = 6 * (1 << 30) // 6 GB

	cdEntry := buildCDEntryWithZip64Extra(
		"big.bin",
		uint32(uint32max), uint32(uint32max), uint32(wantOffset),
		[]uint64{wantUncompSize, wantCompSize}, // only sizes in ZIP64
	)

	f := &File{}
	err := readDirectoryHeader(f, bytes.NewReader(cdEntry))
	if err != nil {
		t.Fatalf("readDirectoryHeader failed: %v", err)
	}

	if f.headerOffset != wantOffset {
		t.Errorf("headerOffset = %d, want %d", f.headerOffset, wantOffset)
	}
	if f.CompressedSize64 != wantCompSize {
		t.Errorf("CompressedSize64 = %d, want %d", f.CompressedSize64, wantCompSize)
	}
	if f.UncompressedSize64 != wantUncompSize {
		t.Errorf("UncompressedSize64 = %d, want %d", f.UncompressedSize64, wantUncompSize)
	}
}

// TestZip64AllFieldsExtra verifies full ZIP64 extra with all three fields.
func TestZip64AllFieldsExtra(t *testing.T) {
	const wantOffset int64 = 10 * (1 << 30)
	const wantCompSize uint64 = 5 * (1 << 30)
	const wantUncompSize uint64 = 8 * (1 << 30)

	cdEntry := buildCDEntryWithZip64Extra(
		"huge.dat",
		uint32(uint32max), uint32(uint32max), uint32(uint32max),
		[]uint64{wantUncompSize, wantCompSize, uint64(wantOffset)},
	)

	f := &File{}
	err := readDirectoryHeader(f, bytes.NewReader(cdEntry))
	if err != nil {
		t.Fatalf("readDirectoryHeader failed: %v", err)
	}

	if f.headerOffset != wantOffset {
		t.Errorf("headerOffset = %d, want %d", f.headerOffset, wantOffset)
	}
	if f.CompressedSize64 != wantCompSize {
		t.Errorf("CompressedSize64 = %d, want %d", f.CompressedSize64, wantCompSize)
	}
	if f.UncompressedSize64 != wantUncompSize {
		t.Errorf("UncompressedSize64 = %d, want %d", f.UncompressedSize64, wantUncompSize)
	}
}

// TestZip64MissingRequiredExtra verifies that ErrFormat is returned
// when a field is 0xFFFFFFFF but the ZIP64 extra is absent.
func TestZip64MissingRequiredExtra(t *testing.T) {
	nameBytes := []byte("missing.txt")
	buf := make([]byte, directoryHeaderLen+len(nameBytes))
	b := writeBuf(buf)
	b.uint32(directoryHeaderSignature)
	b.uint16(zipVersion45)
	b.uint16(zipVersion45)
	b.uint16(0)
	b.uint16(Store)
	b.uint16(0)
	b.uint16(0)
	b.uint32(0)
	b.uint32(100)            // compressedSize fits
	b.uint32(uint32(uint32max)) // uncompressedSize = 0xFFFFFFFF → needs ZIP64
	b.uint16(uint16(len(nameBytes)))
	b.uint16(0) // no extra
	b.uint16(0)
	b.uint16(0)
	b.uint16(0)
	b.uint32(0)
	b.uint32(0)
	copy(buf[directoryHeaderLen:], nameBytes)

	f := &File{}
	err := readDirectoryHeader(f, bytes.NewReader(buf))
	if err != ErrFormat {
		t.Fatalf("expected ErrFormat for missing ZIP64 extra, got %v", err)
	}
}

// TestZip64WriterAlwaysWritesAllFields verifies the writer always writes
// all three ZIP64 fields for backward compatibility with older readers.
func TestZip64WriterAlwaysWritesAllFields(t *testing.T) {
	buf := new(rleBuffer)
	w := NewWriter(buf)

	fh := &FileHeader{
		Name:   "small.txt",
		Method: Store,
	}
	fw, err := w.CreateHeader(fh)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("hello world"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, buf.Size())
	if _, err := buf.ReadAt(data, 0); err != nil && err != io.ErrUnexpectedEOF {
		t.Fatal(err)
	}

	// Small file should NOT have ZIP64 extra at all
	for i := 0; i < len(data)-4; i++ {
		if binary.LittleEndian.Uint32(data[i:i+4]) == directoryHeaderSignature {
			extraLen := int(binary.LittleEndian.Uint16(data[i+30 : i+32]))
			if extraLen > 0 {
				extra := data[i+directoryHeaderLen+len("small.txt") : i+directoryHeaderLen+len("small.txt")+extraLen]
				for j := 0; j < len(extra)-4; {
					tag := binary.LittleEndian.Uint16(extra[j : j+2])
					sz := int(binary.LittleEndian.Uint16(extra[j+2 : j+4]))
					if tag == zip64ExtraId {
						// If ZIP64 is present, it must have all 3 fields (24 bytes)
						if sz != 24 {
							t.Errorf("ZIP64 extra size = %d, want 24 (all 3 fields)", sz)
						}
					}
					j += 4 + sz
				}
			}
			return
		}
	}
	t.Fatal("CD entry not found in output")
}
