package zip

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestZipCryptoStreamDecrypt tests the streaming decryption for ZipCrypto encrypted files.
// This test verifies that streaming decryption produces the same result as buffered decryption.
func TestZipCryptoStreamDecrypt(t *testing.T) {
	file := "zipcrypto-test.zip"
	testFile := filepath.Join("testdata", file)
	password := "123456"

	// Skip if test file doesn't exist
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test file %s not found. Please create a ZipCrypto encrypted zip file for testing.", testFile)
	}

	r, err := OpenReader(testFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	if len(r.File) == 0 {
		t.Fatal("No files in archive")
	}

	for _, f := range r.File {
		if !f.IsEncrypted() {
			continue
		}

		// Test with buffered decryption (original method)
		f.SetPassword(password)
		rc1, err := f.Open()
		if err != nil {
			t.Fatalf("Failed to open file with buffered decryption: %v", err)
		}
		bufferedResult, err := io.ReadAll(rc1)
		rc1.Close()
		if err != nil {
			t.Fatalf("Failed to read with buffered decryption: %v", err)
		}

		// Test with streaming decryption (new method)
		f.SetPassword(password)
		rc2, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open file with streaming decryption: %v", err)
		}
		streamResult, err := io.ReadAll(rc2)
		rc2.Close()
		if err != nil {
			t.Fatalf("Failed to read with streaming decryption: %v", err)
		}

		// Compare results
		if !bytes.Equal(bufferedResult, streamResult) {
			t.Errorf("Streaming decryption result differs from buffered decryption for file %s", f.Name)
			t.Errorf("Buffered length: %d, Stream length: %d", len(bufferedResult), len(streamResult))
			t.Errorf("Buffered: %x", bufferedResult[:min(32, len(bufferedResult))])
			t.Errorf("Stream: %x", streamResult[:min(32, len(streamResult))])
		} else {
			t.Logf("Successfully decrypted %s (%d bytes) using streaming decryption", f.Name, len(streamResult))
		}
	}
}

// TestZipCryptoStreamDecryptChunked tests streaming decryption with chunked reads.
// This simulates how large files would be processed in a memory-efficient manner.
func TestZipCryptoStreamDecryptChunked(t *testing.T) {
	file := "zipcrypto-test.zip"
	testFile := filepath.Join("testdata", file)
	password := "123456"

	// Skip if test file doesn't exist
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test file %s not found. Please create a ZipCrypto encrypted zip file for testing.", testFile)
	}

	r, err := OpenReader(testFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.IsEncrypted() {
			continue
		}

		// Get expected result with buffered decryption
		f.SetPassword(password)
		rc1, err := f.Open()
		if err != nil {
			t.Fatalf("Failed to open file: %v", err)
		}
		expectedResult, err := io.ReadAll(rc1)
		rc1.Close()
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		// Test streaming decryption with small chunks
		f.SetPassword(password)
		rc2, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open file with streaming decryption: %v", err)
		}

		var streamResult bytes.Buffer
		chunkSize := 64 // Small chunk size to test streaming
		buf := make([]byte, chunkSize)
		for {
			n, err := rc2.Read(buf)
			if n > 0 {
				streamResult.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Error reading with streaming decryption: %v", err)
			}
		}
		rc2.Close()

		// Compare results
		if !bytes.Equal(expectedResult, streamResult.Bytes()) {
			t.Errorf("Chunked streaming decryption result differs from buffered decryption for file %s", f.Name)
		} else {
			t.Logf("Successfully decrypted %s (%d bytes) with chunked reads", f.Name, streamResult.Len())
		}
	}
}

// TestStreamDecryptMemoryEfficiency is a test to demonstrate
// memory efficiency of streaming decryption.
// For large files, streaming decryption should use significantly less memory.
func TestStreamDecryptMemoryEfficiency(t *testing.T) {
	file := "zipcrypto-large-test.zip"
	testFile := filepath.Join("testdata", file)
	password := "123456"

	// Skip if test file doesn't exist
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Large test file %s not found. Create a large ZipCrypto encrypted zip file for memory testing.", testFile)
	}

	r, err := OpenReader(testFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.IsEncrypted() {
			continue
		}

		// Use streaming decryption and process in chunks
		f.SetPassword(password)
		rc, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open file with streaming decryption: %v", err)
		}

		// Process file in chunks - this is memory efficient
		totalBytes := int64(0)
		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := rc.Read(buf)
			totalBytes += int64(n)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Error reading: %v", err)
			}
		}
		rc.Close()

		t.Logf("Processed %d bytes from %s using streaming decryption", totalBytes, f.Name)
	}
}

// TestOpenStreamNonEncrypted tests that OpenStream works correctly for non-encrypted files.
func TestOpenStreamNonEncrypted(t *testing.T) {
	// Create a simple non-encrypted zip in memory
	buf := new(bytes.Buffer)
	w := NewWriter(buf)
	content := []byte("Hello, World!")

	fw, err := w.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create file in zip: %v", err)
	}
	_, err = fw.Write(content)
	if err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}
	w.Close()

	// Read it back using OpenStream
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	for _, f := range r.File {
		rc, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open stream: %v", err)
		}
		result, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}
		if !bytes.Equal(content, result) {
			t.Errorf("Content mismatch: expected %s, got %s", content, result)
		}
	}
}

// TestAESStreamDecrypt tests that OpenStream works correctly for AES encrypted files.
// AES decryption is already streaming, so this should work the same as Open().
func TestAESStreamDecrypt(t *testing.T) {
	file := "hello-aes.zip"
	testFile := filepath.Join("testdata", file)

	r, err := OpenReader(testFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.IsEncrypted() {
			continue
		}

		// Test with Open
		f.SetPassword("golang")
		rc1, err := f.Open()
		if err != nil {
			t.Fatalf("Failed to open file: %v", err)
		}
		openResult, err := io.ReadAll(rc1)
		rc1.Close()
		if err != nil {
			t.Fatalf("Failed to read with Open: %v", err)
		}

		// Test with OpenStream
		f.SetPassword("golang")
		rc2, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open file with OpenStream: %v", err)
		}
		streamResult, err := io.ReadAll(rc2)
		rc2.Close()
		if err != nil {
			t.Fatalf("Failed to read with OpenStream: %v", err)
		}

		// Compare results
		if !bytes.Equal(openResult, streamResult) {
			t.Errorf("OpenStream result differs from Open for AES encrypted file %s", f.Name)
		}
	}
}

// TestZipCryptoStreamDecryptInMemory tests streaming decryption using in-memory generated zip.
// This ensures the streaming decryption logic works correctly.
func TestZipCryptoStreamDecryptInMemory(t *testing.T) {
	// Create a ZipCrypto encrypted zip in memory for testing
	contents := []byte("Hello World! This is a test file for streaming decryption testing. " +
		"Adding more content to make it a bit larger and ensure the streaming works properly " +
		"across multiple read operations.")
	password := "testpassword"

	raw := new(bytes.Buffer)
	zipw := NewWriter(raw)
	w, err := zipw.Encrypt("test.txt", password, StandardEncryption)
	if err != nil {
		t.Fatalf("Failed to create encrypted file: %v", err)
	}
	_, err = io.Copy(w, bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("Failed to write contents: %v", err)
	}
	zipw.Close()

	// Open the zip archive
	r, err := NewReader(bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	for _, f := range r.File {
		if !f.IsEncrypted() {
			continue
		}

		// Test with buffered decryption (original method)
		f.SetPassword(password)
		rc1, err := f.Open()
		if err != nil {
			t.Fatalf("Failed to open file with buffered decryption: %v", err)
		}
		bufferedResult, err := io.ReadAll(rc1)
		rc1.Close()
		if err != nil {
			t.Fatalf("Failed to read with buffered decryption: %v", err)
		}

		// Test with streaming decryption (new method)
		f.SetPassword(password)
		rc2, err := f.OpenStream()
		if err != nil {
			t.Fatalf("Failed to open file with streaming decryption: %v", err)
		}
		streamResult, err := io.ReadAll(rc2)
		rc2.Close()
		if err != nil {
			t.Fatalf("Failed to read with streaming decryption: %v", err)
		}

		// Compare results
		if !bytes.Equal(bufferedResult, streamResult) {
			t.Errorf("Streaming decryption result differs from buffered decryption")
			t.Errorf("Buffered length: %d, Stream length: %d", len(bufferedResult), len(streamResult))
		}

		// Verify content matches original
		if !bytes.Equal(contents, streamResult) {
			t.Errorf("Decrypted content doesn't match original. Expected: %s, Got: %s", contents, streamResult)
		}

		t.Logf("In-memory test passed: decrypted %d bytes", len(streamResult))
	}
}

// BenchmarkBufferedDecryption benchmarks the original buffered decryption method.
func BenchmarkBufferedDecryption(b *testing.B) {
	file := "zipcrypto-test.zip"
	testFile := filepath.Join("testdata", file)
	password := "123456"

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skipf("Test file %s not found.", testFile)
	}

	r, err := OpenReader(testFile)
	if err != nil {
		b.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	if len(r.File) == 0 {
		b.Skip("No files in archive")
	}

	f := r.File[0]
	if !f.IsEncrypted() {
		b.Skip("File is not encrypted")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.SetPassword(password)
		rc, err := f.Open()
		if err != nil {
			b.Fatalf("Failed to open: %v", err)
		}
		io.Copy(io.Discard, rc)
		rc.Close()
	}
}

// BenchmarkStreamDecryption benchmarks the streaming decryption method.
func BenchmarkStreamDecryption(b *testing.B) {
	file := "zipcrypto-test.zip"
	testFile := filepath.Join("testdata", file)
	password := "123456"

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skipf("Test file %s not found.", testFile)
	}

	r, err := OpenReader(testFile)
	if err != nil {
		b.Fatalf("Failed to open %s: %v", file, err)
	}
	defer r.Close()

	if len(r.File) == 0 {
		b.Skip("No files in archive")
	}

	f := r.File[0]
	if !f.IsEncrypted() {
		b.Skip("File is not encrypted")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.SetPassword(password)
		rc, err := f.OpenStream()
		if err != nil {
			b.Fatalf("Failed to open: %v", err)
		}
		io.Copy(io.Discard, rc)
		rc.Close()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
