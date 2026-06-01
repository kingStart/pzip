package zip

import (
	"compress/flate"
	"errors"
	"io"
	"sync"
)

type Compressor func(io.Writer) (io.WriteCloser, error)

type Decompressor func(io.Reader) io.ReadCloser

var flateWriterPool sync.Pool

func newFlateWriter(w io.Writer) io.WriteCloser {
	fw, ok := flateWriterPool.Get().(*flate.Writer)
	if ok {
		fw.Reset(w)
	} else {
		fw, _ = flate.NewWriter(w, 5)
	}
	return &pooledFlateWriter{fw: fw}
}

type pooledFlateWriter struct {
	mu sync.Mutex
	fw *flate.Writer
}

func (w *pooledFlateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fw == nil {
		return 0, errors.New("Write after Close")
	}
	return w.fw.Write(p)
}

func (w *pooledFlateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var err error
	if w.fw != nil {
		err = w.fw.Close()
		flateWriterPool.Put(w.fw)
		w.fw = nil
	}
	return err
}

var flateReaderPool sync.Pool

func newFlateReader(r io.Reader) io.ReadCloser {
	fr, ok := flateReaderPool.Get().(io.ReadCloser)
	if ok {
		fr.(flate.Resetter).Reset(r, nil)
	} else {
		fr = flate.NewReader(r)
	}
	return &pooledFlateReader{fr: fr}
}

type pooledFlateReader struct {
	mu sync.Mutex
	fr io.ReadCloser
}

func (r *pooledFlateReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fr == nil {
		return 0, errors.New("Read after Close")
	}
	return r.fr.Read(p)
}

func (r *pooledFlateReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	if r.fr != nil {
		err = r.fr.Close()
		flateReaderPool.Put(r.fr)
		r.fr = nil
	}
	return err
}

var (
	mu sync.RWMutex

	compressors = map[uint16]Compressor{
		Store:   func(w io.Writer) (io.WriteCloser, error) { return &nopCloser{w}, nil },
		Deflate: func(w io.Writer) (io.WriteCloser, error) { return newFlateWriter(w), nil },
	}

	decompressors = map[uint16]Decompressor{
		Store:   io.NopCloser,
		Deflate: newFlateReader,
	}
)

func RegisterDecompressor(method uint16, d Decompressor) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := decompressors[method]; ok {
		panic("decompressor already registered")
	}
	decompressors[method] = d
}

func RegisterCompressor(method uint16, comp Compressor) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := compressors[method]; ok {
		panic("compressor already registered")
	}
	compressors[method] = comp
}

func compressor(method uint16) Compressor {
	mu.RLock()
	defer mu.RUnlock()
	return compressors[method]
}

func decompressor(method uint16) Decompressor {
	mu.RLock()
	defer mu.RUnlock()
	return decompressors[method]
}
