package logger

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type syncBuffer struct {
	data       []byte
	syncCalled int
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *syncBuffer) Sync() error {
	b.syncCalled++
	return nil
}

func TestLoggerFlushesAfterWrite(t *testing.T) {
	out := &syncBuffer{}
	Logger{Out: out}.Debug("msg", []ports.LogField{{Key: "session", Value: "alpha"}})

	if out.syncCalled != 1 {
		t.Fatalf("Sync called %d times, want 1", out.syncCalled)
	}
}
