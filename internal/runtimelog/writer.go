package runtimelog

import (
	"os"
	"path/filepath"
	"sync"
)

// Writer appends to a bounded runtime log file.
//
// When a single write exceeds maxBytes, Writer keeps only the tail that fits in
// the bounded log while still treating the original payload as consumed so
// callers like log.Logger do not retry partial log entries.
type Writer struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

func NewWriter(path string, maxBytes int64) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Writer{path: path, maxBytes: maxBytes, file: file, size: info.Size()}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	written := len(p)
	if w.maxBytes > 0 && int64(len(p)) > w.maxBytes {
		start := int(int64(len(p)) - w.maxBytes)
		p = p[start:]
	}
	if w.maxBytes > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return written, err
	}
	return written, nil
}

func (w *Writer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.closeLocked()
}

func (w *Writer) rotateLocked() error {
	if w.file != nil {
		if err := w.closeLocked(); err != nil {
			return err
		}
	}
	rotated := w.path + ".1"
	if err := os.Remove(rotated); err != nil && !os.IsNotExist(err) {
		return err
	}
	if w.size > 0 {
		if err := os.Rename(w.path, rotated); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	w.size = 0
	return nil
}

func (w *Writer) closeLocked() error {
	syncErr := w.file.Sync()
	closeErr := w.file.Close()
	w.file = nil
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
