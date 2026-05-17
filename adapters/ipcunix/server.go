package ipcunix

import (
	"context"
	"encoding/json"
	"fmt"
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
	if err := removeStaleSocket(s.SocketPath); err != nil {
		return err
	}
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

func removeStaleSocket(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket IPC path %s", path)
	}
	return os.Remove(path)
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
	ctxWithDeadline, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	resp, err := handler.HandleIPC(ctxWithDeadline, req)
	if err != nil {
		resp = ports.Response{OK: false, Message: err.Error()}
	}
	_ = conn.SetWriteDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		log.Printf("tmux-session-sidebar ipc encode response for %q failed: %v", req.Kind, err)
	}
}
