package ipcunix

import (
	"context"
	"encoding/json"
	"net"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Client struct {
	SocketPath string
}

func NewClient(socketPath string) Client { return Client{SocketPath: socketPath} }

func (c Client) Send(ctx context.Context, req ports.Request) (ports.Response, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return ports.Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return ports.Response{}, err
	}
	var resp ports.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return ports.Response{}, err
	}
	return resp, nil
}
