package ipcunix

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestIPCUnixRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  ports.Request
		resp ports.Response
	}{
		{name: "simple request", req: ports.Request{Kind: "ping", ClientID: "%1"}, resp: ports.Response{OK: true, Message: "pong"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			socket := filepath.Join(t.TempDir(), "sock")
			server := NewServer(socket)
			handler := mocks.NewMockIPCHandler(t)
			handler.EXPECT().HandleIPC(mock.Anything, tt.req).Return(tt.resp, nil)

			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx, handler) }()

			got, err := sendWithRetry(ctx, socket, tt.req, time.Second)
			if err != nil {
				t.Fatalf("Send error: %v", err)
			}
			if got.OK != tt.resp.OK || got.Message != tt.resp.Message {
				t.Fatalf("response = %#v, want %#v", got, tt.resp)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("server error: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
			}
		})
	}
}

func TestServeRefusesToRemoveNonSocketPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	handler := mocks.NewMockIPCHandler(t)
	if err := NewServer(path).Serve(context.Background(), handler); err == nil {
		t.Fatal("Serve error = nil, want error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("regular file was removed: %v", err)
	}
}

func sendWithRetry(ctx context.Context, socket string, req ports.Request, timeout time.Duration) (ports.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		resp, err := NewClient(socket).Send(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			_ = lastErr
			return ports.Response{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
