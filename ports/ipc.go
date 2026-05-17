package ports

import "context"

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

type IPCHandler interface {
	HandleIPC(ctx context.Context, req Request) (Response, error)
}

type IPCClientPort interface {
	Send(ctx context.Context, req Request) (Response, error)
}

type IPCServerPort interface {
	Serve(ctx context.Context, handler IPCHandler) error
}
