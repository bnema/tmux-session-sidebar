package app

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type recordingIPCRouter struct {
	routes []Route
	err    error
}

type blockingIPCRouter struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingIPCRouter) Handle(_ context.Context, _ Route, _ io.Writer, _ io.Writer) error {
	r.entered <- struct{}{}
	<-r.release
	return nil
}

func (r *recordingIPCRouter) Handle(_ context.Context, route Route, _ io.Writer, _ io.Writer) error {
	r.routes = append(r.routes, route)
	return r.err
}

func TestDaemonIPCHandlerDispatchesSidebarRequests(t *testing.T) {
	tests := []struct {
		name      string
		req       ports.Request
		wantRoute string
	}{
		{name: "open", req: ports.SidebarOpenRequest("%1"), wantRoute: "sidebar/open"},
		{name: "close", req: ports.SidebarCloseRequest("%1"), wantRoute: "sidebar/close"},
		{name: "toggle", req: ports.SidebarToggleRequest("%1"), wantRoute: "sidebar/toggle"},
		{name: "refresh", req: ports.SidebarRefreshRequest("%1"), wantRoute: "sidebar/refresh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &recordingIPCRouter{}
			resp, err := daemonIPCHandler{router: router}.HandleIPC(t.Context(), tt.req)
			if err != nil {
				t.Fatalf("HandleIPC error: %v", err)
			}
			if !resp.OK {
				t.Fatalf("response OK = false, message=%q", resp.Message)
			}
			if len(router.routes) != 1 {
				t.Fatalf("routes = %d, want 1", len(router.routes))
			}
			got := router.routes[0]
			if got.Path != tt.wantRoute {
				t.Fatalf("route path = %q, want %q", got.Path, tt.wantRoute)
			}
			if got.Flags["client"] != "%1" {
				t.Fatalf("client flag = %q, want %%1", got.Flags["client"])
			}
		})
	}
}

func TestDaemonIPCHandlerPreservesSidebarOpenWidth(t *testing.T) {
	router := &recordingIPCRouter{}
	resp, err := daemonIPCHandler{router: router}.HandleIPC(t.Context(), ports.SidebarOpenRequest("%1", "30"))
	if err != nil {
		t.Fatalf("HandleIPC error: %v", err)
	}
	if !resp.OK {
		t.Fatalf("response OK = false, message=%q", resp.Message)
	}
	if len(router.routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(router.routes))
	}
	if router.routes[0].Flags["width"] != "30" {
		t.Fatalf("width flag = %q, want 30", router.routes[0].Flags["width"])
	}
}

func TestDaemonIPCHandlerHandlesHealthAndUnknownRequests(t *testing.T) {
	health, err := (daemonIPCHandler{}).HandleIPC(t.Context(), ports.HealthRequest())
	if err != nil || !health.OK {
		t.Fatalf("health response = %#v, err=%v", health, err)
	}

	resp, err := (daemonIPCHandler{}).HandleIPC(t.Context(), ports.Request{Kind: "unknown"})
	if err == nil {
		t.Fatal("unknown request error = nil, want error")
	}
	if resp.OK {
		t.Fatalf("unknown response OK = true")
	}
}

func TestDaemonIPCHandlerSerializesSidebarMutations(t *testing.T) {
	router := &blockingIPCRouter{entered: make(chan struct{}, 2), release: make(chan struct{})}
	handler := daemonIPCHandler{router: router, mu: &sync.Mutex{}}
	ctx := t.Context()

	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, _ = handler.HandleIPC(ctx, ports.SidebarOpenRequest("%1"))
		}()
	}

	<-router.entered
	select {
	case <-router.entered:
		t.Fatal("second sidebar mutation entered router before first completed")
	default:
	}
	close(router.release)
	wg.Wait()
}

func TestDaemonIPCHandlerReturnsRouteErrors(t *testing.T) {
	boom := errors.New("tmux target missing")
	router := &recordingIPCRouter{err: boom}
	resp, err := (daemonIPCHandler{router: router}).HandleIPC(t.Context(), ports.SidebarOpenRequest("%missing"))
	if !errors.Is(err, boom) {
		t.Fatalf("HandleIPC error = %v, want %v", err, boom)
	}
	if resp.OK {
		t.Fatalf("response OK = true, want false")
	}
}
