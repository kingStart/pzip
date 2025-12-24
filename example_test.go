// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"

	zip "github.com/kingStart/pzip"
)

func ExampleWriter() {
	// Create a buffer to write our archive to.
	buf := new(bytes.Buffer)

	// Create a new zip archive.
	w := zip.NewWriter(buf)

	// Add some files to the archive.
	var files = []struct {
		Name, Body string
	}{
		{"readme.txt", "This archive contains some text files."},
		{"gopher.txt", "Gopher names:\nGeorge\nGeoffrey\nGonzo"},
		{"todo.txt", "Get animal handling licence.\nWrite more examples."},
	}
	for _, file := range files {
		f, err := w.Create(file.Name)
		if err != nil {
			log.Fatal(err)
		}
		_, err = f.Write([]byte(file.Body))
		if err != nil {
			log.Fatal(err)
		}
	}

	// Make sure to check the error on Close.
	err := w.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleReader() {
	// Open a zip archive for reading.
	r, err := zip.OpenReader("testdata/readme.zip")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	// Iterate through the files in the archive,
	// printing some of their contents.
	for _, f := range r.File {
		fmt.Printf("Contents of %s:\n", f.Name)
		rc, err := f.Open()
		if err != nil {
			log.Fatal(err)
		}
		_, err = io.CopyN(os.Stdout, rc, 68)
		if err != nil {
			log.Fatal(err)
		}
		rc.Close()
		fmt.Println()
	}
	// Output:
	// Contents of README:
	// This is the source code repository for the Go programming language.
}

func ExampleWriter_Encrypt() {
	contents := []byte("Hello World")

	// write a password zip
	raw := new(bytes.Buffer)
	zipw := zip.NewWriter(raw)
	w, err := zipw.Encrypt("hello.txt", "golang", zip.AES256Encryption)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(w, bytes.NewReader(contents))
	if err != nil {
		log.Fatal(err)
	}
	zipw.Close()

	// read the password zip
	zipr, err := zip.NewReader(bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		log.Fatal(err)
	}
	for _, z := range zipr.File {
		z.SetPassword("golang")
		rr, err := z.Open()
		if err != nil {
			log.Fatal(err)
		}
		_, err = io.Copy(os.Stdout, rr)
		if err != nil {
			log.Fatal(err)
		}
		rr.Close()
	}
	// Output:
	// Hello World
}

// ExampleOpenStream demonstrates streaming decryption for large encrypted files.
// This method is memory-efficient as it decrypts data on-the-fly without loading
// the entire file into memory. Ideal for large files (e.g., 50GB+).
func ExampleFile_OpenStream() {
	contents := []byte("Hello World - Streaming Decryption Example")

	// Create an encrypted zip using ZipCrypto (StandardEncryption)
	raw := new(bytes.Buffer)
	zipw := zip.NewWriter(raw)
	w, err := zipw.Encrypt("hello.txt", "golang", zip.StandardEncryption)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(w, bytes.NewReader(contents))
	if err != nil {
		log.Fatal(err)
	}
	zipw.Close()

	// Read using streaming decryption - memory efficient for large files
	zipr, err := zip.NewReader(bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		log.Fatal(err)
	}
	for _, z := range zipr.File {
		z.SetPassword("golang")
		// Use OpenStream() instead of Open() for streaming decryption
		rr, err := z.OpenStream()
		if err != nil {
			log.Fatal(err)
		}
		_, err = io.Copy(os.Stdout, rr)
		if err != nil {
			log.Fatal(err)
		}
		rr.Close()
	}
	// Output:
	// Hello World - Streaming Decryption Example
}

// ExampleFile_OpenStream_chunked demonstrates processing large files in chunks.
// This pattern is useful when you need precise control over memory usage.
func ExampleFile_OpenStream_chunked() {
	contents := []byte("Chunked streaming decryption example data")

	// Create an encrypted zip
	raw := new(bytes.Buffer)
	zipw := zip.NewWriter(raw)
	w, err := zipw.Encrypt("data.bin", "password123", zip.StandardEncryption)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(w, bytes.NewReader(contents))
	if err != nil {
		log.Fatal(err)
	}
	zipw.Close()

	// Read in chunks - useful for very large files
	zipr, err := zip.NewReader(bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		log.Fatal(err)
	}
	for _, z := range zipr.File {
		z.SetPassword("password123")
		rr, err := z.OpenStream()
		if err != nil {
			log.Fatal(err)
		}

		// Process in fixed-size chunks (e.g., 16 bytes for demo, use 32KB+ in production)
		buf := make([]byte, 16)
		for {
			n, err := rr.Read(buf)
			if n > 0 {
				fmt.Print(string(buf[:n]))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
		}
		rr.Close()
	}
	// Output:
	// Chunked streaming decryption example data
}
