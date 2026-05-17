package ipcunix

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Server struct {
	SocketPath string
}

func NewServer(socketPath string) Server { return Server{SocketPath: socketPath} }

func (s Server) Serve(ctx context.Context, handler ports.IPCHandler) error {
	_ = os.Remove(s.SocketPath)
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go handleConn(ctx, conn, handler)
	}
}

func handleConn(ctx context.Context, conn net.Conn, handler ports.IPCHandler) {
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = conn.SetReadDeadline(deadline)
	var req ports.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	resp, err := handler.HandleIPC(ctx, req)
	if err != nil {
		resp = ports.Response{OK: false, Message: err.Error()}
	}
	_ = conn.SetWriteDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		log.Printf("tmux-session-sidebar ipc encode response for %q failed: %v", req.Kind, err)
	}
}
