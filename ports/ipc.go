package ports

import (
	"context"
	"maps"
)

const (
	IPCSidebarOpen    = "sidebar.open"
	IPCSidebarClose   = "sidebar.close"
	IPCSidebarToggle  = "sidebar.toggle"
	IPCSidebarRefresh = "sidebar.refresh"
	IPCActiveClient   = "sidebar.active-client"
	IPCHealth         = "daemon.health"
	IPCShutdown       = "daemon.shutdown"
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

func SidebarOpenRequest(clientID string) Request {
	return SidebarRequest(IPCSidebarOpen, clientID, nil)
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

func HealthRequest() Request {
	return Request{Kind: IPCHealth}
}

func ShutdownRequest() Request {
	return Request{Kind: IPCShutdown}
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
