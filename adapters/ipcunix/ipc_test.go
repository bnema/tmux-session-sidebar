package ipcunix

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
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
			handler.EXPECT().HandleIPC(ctx, tt.req).Return(tt.resp, nil)

			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx, handler) }()
			time.Sleep(20 * time.Millisecond)

			got, err := NewClient(socket).Send(ctx, tt.req)
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
