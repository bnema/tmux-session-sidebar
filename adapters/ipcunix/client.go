package ipcunix

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Client struct {
	SocketPath string
}

func NewClient(socketPath string) Client { return Client{SocketPath: socketPath} }

func (c Client) Send(ctx context.Context, req ports.Request) (resp ports.Response, retErr error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return ports.Response{}, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return ports.Response{}, err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return ports.Response{}, err
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return ports.Response{}, err
	}
	return resp, nil
}
