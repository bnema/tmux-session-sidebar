package app

import (
	"io"
	"os"
	"sync"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func newRotatingLogWriter(path string, maxBytes int64) (ports.SyncWriteCloser, error) {
	return runtimeLogWriter(path, maxBytes)
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
