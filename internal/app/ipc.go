package app

import (
	"context"
	"fmt"
	"io"
	"maps"
	"sync"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type daemonIPCHandler struct {
	router Router
	stdout io.Writer
	stderr io.Writer
	mu     *sync.Mutex
}

func (h daemonIPCHandler) HandleIPC(ctx context.Context, req ports.Request) (ports.Response, error) {
	if req.Kind == ports.IPCHealth {
		return ports.Response{OK: true, Message: "ok"}, nil
	}
	if req.Kind == ports.IPCActiveClient {
		return ports.Response{OK: true, Message: "ok"}, nil
	}
	path, ok := ipcRoutePath(req.Kind)
	if !ok {
		err := fmt.Errorf("unknown IPC request kind %q", req.Kind)
		return ports.Response{OK: false, Message: err.Error()}, err
	}
	if h.router == nil {
		err := fmt.Errorf("missing IPC router")
		return ports.Response{OK: false, Message: err.Error()}, err
	}
	if h.mu != nil && ipcRouteMutatesSidebar(req.Kind) {
		h.mu.Lock()
		defer h.mu.Unlock()
	}
	flags := cloneIPCArgs(req.Args)
	if req.ClientID != "" {
		flags["client"] = req.ClientID
	}
	if err := h.router.Handle(ctx, Route{Path: path, Flags: flags}, h.stdout, h.stderr); err != nil {
		return ports.Response{OK: false, Message: err.Error()}, err
	}
	return ports.Response{OK: true, Message: "ok"}, nil
}

func ipcRouteMutatesSidebar(kind string) bool {
	switch kind {
	case ports.IPCSidebarOpen, ports.IPCSidebarClose, ports.IPCSidebarToggle:
		return true
	default:
		return false
	}
}

func ipcRoutePath(kind string) (string, bool) {
	switch kind {
	case ports.IPCSidebarOpen:
		return "sidebar/open", true
	case ports.IPCSidebarClose:
		return "sidebar/close", true
	case ports.IPCSidebarToggle:
		return "sidebar/toggle", true
	case ports.IPCSidebarRefresh:
		return "sidebar/refresh", true
	default:
		return "", false
	}
}

func cloneIPCArgs(args map[string]string) map[string]string {
	cloned := make(map[string]string, len(args)+1)
	maps.Copy(cloned, args)
	return cloned
}
