package app

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type recordingIPCRouter struct {
	mu     sync.Mutex
	routes []Route
	err    error
}

type recordingIPCClient struct {
	requests []ports.Request
	err      error
}

func (c *recordingIPCClient) Send(_ context.Context, req ports.Request) (ports.Response, error) {
	c.requests = append(c.requests, req)
	return ports.Response{OK: true}, c.err
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, route)
	return r.err
}

func (r *recordingIPCRouter) routeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.routes)
}

func (r *recordingIPCRouter) routeAt(index int) Route {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.routes[index]
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

func TestDaemonIPCHandlerSignalsMetadataReconcile(t *testing.T) {
	reconcile := make(chan struct{}, 1)
	resp, err := (daemonIPCHandler{metadataReconcile: reconcile}).HandleIPC(t.Context(), ports.MetadataReconcileRequest())
	if err != nil || !resp.OK {
		t.Fatalf("metadata reconcile response = %#v, err=%v", resp, err)
	}
	select {
	case <-reconcile:
	default:
		t.Fatal("metadata reconcile signal was not sent")
	}
}

func TestRuntimeRouterSendsSessionHookEventsOverIPC(t *testing.T) {
	ipc := &recordingIPCClient{}
	router := NewRuntimeRouterWithDaemonEnvironment(RuntimeEnvironment{}, nil, ipc, nil, nil)

	err := router.Handle(t.Context(), Route{Path: "hook/client-session-changed", Flags: map[string]string{"client": "/dev/pts/4", "session": "alpha"}}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Handle hook route error: %v", err)
	}
	if len(ipc.requests) != 1 {
		t.Fatalf("IPC requests = %d, want 1", len(ipc.requests))
	}
	got := ipc.requests[0]
	if got.Kind != ports.IPCHookClientSessionChanged || got.ClientID != "/dev/pts/4" || got.Args["session"] != "alpha" {
		t.Fatalf("IPC request = %#v", got)
	}
}

func TestDaemonIPCHandlerQueuesRuntimeHookEventsWithoutDispatching(t *testing.T) {
	events := make(chan ports.Request, 1)
	router := &recordingIPCRouter{}
	req := ports.ClientSessionChangedEventRequest("/dev/pts/4", "")

	resp, err := (daemonIPCHandler{router: router, runtimeEvents: events}).HandleIPC(t.Context(), req)
	if err != nil || !resp.OK {
		t.Fatalf("HandleIPC response = %#v, err=%v", resp, err)
	}
	select {
	case got := <-events:
		if got.Kind != req.Kind || got.ClientID != req.ClientID {
			t.Fatalf("queued event = %#v, want %#v", got, req)
		}
	default:
		t.Fatal("runtime event was not queued")
	}
	if len(router.routes) != 0 {
		t.Fatalf("routes dispatched synchronously = %d, want 0", len(router.routes))
	}
}

func TestDaemonIPCHandlerRejectsStaleTmuxServerScope(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t111\n"})
	oldRunner := commandRunner
	commandRunner = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "/tmp/tmux/default\t222\n", nil
	}
	t.Cleanup(func() { commandRunner = oldRunner })

	router := &recordingIPCRouter{}
	resp, err := (daemonIPCHandler{router: router, expectedScope: scope}).HandleIPC(t.Context(), ports.SidebarToggleRequest("/dev/pts/0"))
	if err == nil {
		t.Fatal("HandleIPC error = nil, want stale scope error")
	}
	if resp.OK {
		t.Fatalf("response OK = true, want false")
	}
	if len(router.routes) != 0 {
		t.Fatalf("routes = %d, want no dispatch to stale daemon router", len(router.routes))
	}
}

func TestDaemonIPCHandlerRejectsHealthForStaleTmuxServerScope(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t111\n"})
	oldRunner := commandRunner
	commandRunner = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "/tmp/tmux/default\t222\n", nil
	}
	t.Cleanup(func() { commandRunner = oldRunner })

	resp, err := (daemonIPCHandler{expectedScope: scope}).HandleIPC(t.Context(), ports.HealthRequest())
	if err == nil {
		t.Fatal("HandleIPC health error = nil, want stale scope error")
	}
	if resp.OK {
		t.Fatalf("health response OK = true, want false")
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

func TestDaemonRuntimeEventProcessorCoalescesSessionChangedStorm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ports.Request, 32)
	router := &recordingIPCRouter{}
	done := make(chan error, 1)
	go func() {
		done <- runDaemonRuntimeEventProcessor(ctx, daemonRuntimeEventProcessorOptions{
			router:   router,
			events:   events,
			debounce: 10 * time.Millisecond,
			ready:    func(context.Context) bool { return true },
		})
	}()

	for range 25 {
		events <- ports.ClientSessionChangedEventRequest("/dev/pts/4", "")
	}

	assertEventually(t, time.Second, func() bool { return router.routeCount() == 1 })
	got := router.routeAt(0)
	if got.Path != "hook/client-session-changed" || got.Flags["client"] != "/dev/pts/4" {
		t.Fatalf("coalesced route = %#v", got)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("processor error: %v", err)
	}
}

func TestDaemonRuntimeEventProcessorPreservesMixedClientAndKindEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ports.Request, 32)
	router := &recordingIPCRouter{}
	done := make(chan error, 1)
	go func() {
		done <- runDaemonRuntimeEventProcessor(ctx, daemonRuntimeEventProcessorOptions{
			router:   router,
			events:   events,
			debounce: 10 * time.Millisecond,
			ready:    func(context.Context) bool { return true },
		})
	}()

	events <- ports.ClientSessionChangedEventRequest("/dev/pts/4", "alpha")
	events <- ports.ClientSessionChangedEventRequest("/dev/pts/5", "beta")
	events <- ports.ClientAttachedEventRequest("/dev/pts/4", "alpha")
	events <- ports.ClientSessionChangedEventRequest("/dev/pts/4", "alpha-new")

	assertEventually(t, time.Second, func() bool { return router.routeCount() == 3 })
	got := map[string]Route{}
	for i := range 3 {
		route := router.routeAt(i)
		got[route.Path+"\x00"+route.Flags["client"]] = route
	}
	if got["hook/client-session-changed\x00/dev/pts/4"].Flags["session"] != "alpha-new" {
		t.Fatalf("coalesced /dev/pts/4 session-changed route = %#v", got["hook/client-session-changed\x00/dev/pts/4"])
	}
	if got["hook/client-session-changed\x00/dev/pts/5"].Flags["session"] != "beta" {
		t.Fatalf("coalesced /dev/pts/5 session-changed route = %#v", got["hook/client-session-changed\x00/dev/pts/5"])
	}
	if got["hook/client-attached\x00/dev/pts/4"].Flags["session"] != "alpha" {
		t.Fatalf("attached route = %#v", got["hook/client-attached\x00/dev/pts/4"])
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("processor error: %v", err)
	}
}

func TestRuntimeEventsReadyAfterRestoreDoesNotBlockWhenSessionRestoreIsOff(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-restore-sessions) printf 'off\n' ;;
      @continuum-restore) printf 'on\n' ;;
      @continuum-restore-max-delay) printf '999\n' ;;
      @resurrect-restore-script-path) printf '/tmp/resurrect/restore.sh\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) date +%s ;;
esac
`)

	if !runtimeEventsReadyAfterRestore(t.Context()) {
		t.Fatal("runtime events were blocked even though tmux-session-sidebar restore is disabled")
	}
}

func TestDaemonRuntimeEventProcessorWaitsUntilRestoreReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ports.Request, 8)
	router := &recordingIPCRouter{}
	var ready atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- runDaemonRuntimeEventProcessor(ctx, daemonRuntimeEventProcessorOptions{
			router:   router,
			events:   events,
			debounce: 10 * time.Millisecond,
			ready:    func(context.Context) bool { return ready.Load() },
		})
	}()

	events <- ports.ClientSessionChangedEventRequest("/dev/pts/4", "")
	time.Sleep(40 * time.Millisecond)
	if count := router.routeCount(); count != 0 {
		t.Fatalf("routes while restore not ready = %d, want 0", count)
	}
	ready.Store(true)
	assertEventually(t, time.Second, func() bool { return router.routeCount() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("processor error: %v", err)
	}
}

func assertEventually(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
