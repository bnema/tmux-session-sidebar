package ipcunix

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestWrapIPCClientError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
	}{
		{name: "missing socket", err: os.ErrNotExist, target: ports.ErrIPCSocketMissing},
		{name: "connection refused", err: syscall.ECONNREFUSED, target: ports.ErrIPCConnectionRefused},
		{name: "connection reset", err: syscall.ECONNRESET, target: ports.ErrIPCConnectionReset},
		{name: "other error", err: errors.New("boom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapIPCClientError(tt.err)
			if tt.target == nil {
				if got != tt.err {
					t.Fatalf("wrapIPCClientError() returned %v, want original %v", got, tt.err)
				}
				return
			}
			if !errors.Is(got, tt.target) {
				t.Fatalf("wrapIPCClientError() error = %v, want errors.Is(..., %v)", got, tt.target)
			}
		})
	}
}
