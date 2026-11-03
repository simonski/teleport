package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sync"
)

// webFS inflates each embedded asset at most once; the binary retains only the
// compressed archive, while FileServer receives ordinary seekable files.
type webFS struct {
	archive *zip.Reader
	cache   sync.Map
}

type webFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *webFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *webFile) Close() error               { return nil }

type webContent struct {
	data []byte
	info fs.FileInfo
}

func newWebFS(data []byte) *webFS {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		panic(fmt.Errorf("invalid embedded web assets: %w", err))
	}
	return &webFS{archive: archive}
}

func (w *webFS) Open(name string) (fs.File, error) {
	if cached, ok := w.cache.Load(name); ok {
		content := cached.(webContent)
		return &webFile{Reader: bytes.NewReader(content.data), info: content.info}, nil
	}
	file, err := w.archive.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.IsDir() {
		return file, nil
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	content, _ := w.cache.LoadOrStore(name, webContent{data: data, info: info})
	stored := content.(webContent)
	return &webFile{Reader: bytes.NewReader(stored.data), info: stored.info}, nil
}
