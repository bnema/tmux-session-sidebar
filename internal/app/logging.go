package app

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

type rotatingLogWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

func newRotatingLogWriter(path string, maxBytes int64) (*rotatingLogWriter, error) {
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
	return &rotatingLogWriter{path: path, maxBytes: maxBytes, file: file, size: info.Size()}, nil
}

func (w *rotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written := len(p)
	if w.maxBytes > 0 && int64(len(p)) > w.maxBytes {
		p = p[len(p)-int(w.maxBytes):]
	}
	if w.maxBytes > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return n, err
	}
	return written, nil
}

func (w *rotatingLogWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *rotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	_ = w.file.Sync()
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingLogWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Sync()
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
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
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	w.size = 0
	return nil
}

func redirectStderrToRotatingLog(path string, maxBytes int64) (func(), error) {
	writer, err := newRotatingLogWriter(path, maxBytes)
	if err != nil {
		return nil, err
	}
	reader, pipeWriter, err := os.Pipe()
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	previous := os.Stderr
	os.Stderr = pipeWriter
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(writer, reader)
		_ = reader.Close()
		_ = writer.Close()
		close(done)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			os.Stderr = previous
			_ = pipeWriter.Close()
			<-done
		})
	}, nil
}
