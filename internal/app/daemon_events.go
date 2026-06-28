package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

const defaultRuntimeEventDebounce = 250 * time.Millisecond

type daemonRuntimeEventProcessorOptions struct {
	router            Router
	events            <-chan ports.Request
	debounce          time.Duration
	ready             func(context.Context) bool
	stdout            io.Writer
	stderr            io.Writer
	sidebarMutationMu *sync.Mutex
}

func runDaemonRuntimeEventProcessor(ctx context.Context, opts daemonRuntimeEventProcessorOptions) error {
	if opts.events == nil || opts.router == nil {
		<-ctx.Done()
		return nil
	}
	if opts.debounce <= 0 {
		opts.debounce = defaultRuntimeEventDebounce
	}
	if opts.ready == nil {
		opts.ready = func(context.Context) bool { return true }
	}
	if opts.stdout == nil {
		opts.stdout = io.Discard
	}
	if opts.stderr == nil {
		opts.stderr = os.Stderr
	}

	for {
		pending, ok := waitRuntimeEvent(ctx, opts.events)
		if !ok {
			return nil
		}
		batch := coalesceRuntimeEvents(ctx, opts.events, newRuntimeEventBatch(pending), opts.debounce)
		for !opts.ready(ctx) {
			select {
			case <-ctx.Done():
				return nil
			case event, ok := <-opts.events:
				if !ok {
					return nil
				}
				batch.add(event)
			case <-time.After(opts.debounce):
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		for _, route := range batch.routes() {
			if ctx.Err() != nil {
				return nil
			}
			if err := handleRuntimeEventRoute(ctx, opts, route); err != nil {
				_, _ = fmt.Fprintf(opts.stderr, "tmux-session-sidebar: runtime event %s failed: %v\n", route.Path, err)
			}
		}
	}
}

func handleRuntimeEventRoute(ctx context.Context, opts daemonRuntimeEventProcessorOptions, route Route) error {
	if opts.sidebarMutationMu != nil && directRouteMutatesSidebar(route.Path) {
		opts.sidebarMutationMu.Lock()
		defer opts.sidebarMutationMu.Unlock()
	}
	return opts.router.Handle(ctx, route, opts.stdout, opts.stderr)
}

func waitRuntimeEvent(ctx context.Context, events <-chan ports.Request) (ports.Request, bool) {
	select {
	case <-ctx.Done():
		return ports.Request{}, false
	case event, ok := <-events:
		return event, ok
	}
}

func coalesceRuntimeEvents(ctx context.Context, events <-chan ports.Request, pending runtimeEventBatch, debounce time.Duration) runtimeEventBatch {
	timer := time.NewTimer(debounce)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return pending
		case event, ok := <-events:
			if !ok {
				return pending
			}
			pending.add(event)
			resetTimer(timer, debounce)
		case <-timer.C:
			return pending
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

type runtimeEventKey struct {
	kind   string
	client string
}

type runtimeEventBatch struct {
	order  []runtimeEventKey
	events map[runtimeEventKey]ports.Request
}

func newRuntimeEventBatch(first ports.Request) runtimeEventBatch {
	batch := runtimeEventBatch{events: map[runtimeEventKey]ports.Request{}}
	batch.add(first)
	return batch
}

func (b *runtimeEventBatch) add(event ports.Request) {
	if !ipcRuntimeEvent(event.Kind) {
		return
	}
	key := runtimeEventKey{kind: event.Kind, client: event.ClientID}
	if _, ok := b.events[key]; !ok {
		b.order = append(b.order, key)
	}
	b.events[key] = event
}

func (b runtimeEventBatch) routes() []Route {
	routes := make([]Route, 0, len(b.order))
	for _, key := range b.order {
		route, ok := runtimeEventRoute(b.events[key])
		if ok {
			routes = append(routes, route)
		}
	}
	return routes
}

func runtimeEventRoute(req ports.Request) (Route, bool) {
	path, ok := runtimeEventRoutePath(req.Kind)
	if !ok {
		return Route{}, false
	}
	flags := cloneIPCArgs(req.Args)
	if req.ClientID != "" {
		flags["client"] = req.ClientID
	}
	return Route{Path: path, Flags: flags}, true
}

func runtimeEventRoutePath(kind string) (string, bool) {
	switch kind {
	case ports.IPCHookClientAttached:
		return "hook/client-attached", true
	case ports.IPCHookClientDetached:
		return "hook/client-detached", true
	case ports.IPCHookClientSessionChanged:
		return "hook/client-session-changed", true
	default:
		return "", false
	}
}

func runtimeEventsReadyAfterRestore(ctx context.Context) bool {
	cfg := loadSidebarConfig(ctx)
	return !continuumRestoreInProgress(ctx, cfg)
}
