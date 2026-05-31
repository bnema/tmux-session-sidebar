package ports

import (
	"context"
	"errors"
	"maps"
	"strings"
)

const (
	IPCSidebarOpen       = "sidebar.open"
	IPCSidebarClose      = "sidebar.close"
	IPCSidebarToggle     = "sidebar.toggle"
	IPCSidebarRefresh    = "sidebar.refresh"
	IPCMetadataReconcile = "metadata.reconcile"
	IPCActiveClient      = "sidebar.active-client"
	IPCHealth            = "daemon.health"
)

var (
	ErrIPCSocketMissing     = errors.New("ipc socket missing")
	ErrIPCConnectionRefused = errors.New("ipc connection refused")
	ErrIPCConnectionReset   = errors.New("ipc connection reset")
)

type Request struct {
	Kind     string
	ClientID string
	Args     map[string]string
}

type Response struct {
	OK      bool
	Message string
	Payload []byte
}

func SidebarRequest(kind string, clientID string, args map[string]string) Request {
	cloned := map[string]string(nil)
	if len(args) > 0 {
		cloned = make(map[string]string, len(args))
		maps.Copy(cloned, args)
	}
	return Request{Kind: kind, ClientID: clientID, Args: cloned}
}

func SidebarOpenRequest(clientID string, width ...string) Request {
	var args map[string]string
	if len(width) > 0 {
		trimmed := strings.TrimSpace(width[0])
		if trimmed != "" {
			args = map[string]string{"width": trimmed}
		}
	}
	return SidebarRequest(IPCSidebarOpen, clientID, args)
}

func SidebarCloseRequest(clientID string) Request {
	return SidebarRequest(IPCSidebarClose, clientID, nil)
}

func SidebarToggleRequest(clientID string) Request {
	return SidebarRequest(IPCSidebarToggle, clientID, nil)
}

func SidebarRefreshRequest(clientID string) Request {
	return SidebarRequest(IPCSidebarRefresh, clientID, nil)
}

func ActiveClientRequest(clientID string) Request {
	return SidebarRequest(IPCActiveClient, clientID, nil)
}

func MetadataReconcileRequest() Request {
	return Request{Kind: IPCMetadataReconcile}
}

func HealthRequest() Request {
	return Request{Kind: IPCHealth}
}

type IPCHandler interface {
	HandleIPC(ctx context.Context, req Request) (Response, error)
}

type IPCClientPort interface {
	Send(ctx context.Context, req Request) (Response, error)
}

type IPCServerPort interface {
	Serve(ctx context.Context, handler IPCHandler) error
}
