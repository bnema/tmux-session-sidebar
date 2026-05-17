package locker

import (
	"context"
	"os"
	"path/filepath"
)

type FileLocker struct {
	Dir string
}

type Handle struct {
	path string
	file *os.File
}

func (l FileLocker) Acquire(_ context.Context, key string) (*Handle, error) {
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(l.Dir, key+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &Handle{path: path, file: file}, nil
}

func (h *Handle) Release() error {
	if h.file != nil {
		_ = h.file.Close()
	}
	return os.Remove(h.path)
}
