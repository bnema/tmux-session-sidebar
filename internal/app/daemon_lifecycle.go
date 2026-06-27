package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type daemonLifecycleCoordinator struct {
	env       RuntimeEnvironment
	scope     RuntimeScope
	ipcServer ports.IPCServerPort
	router    Router
	opts      daemonServeOptions
}

func newDaemonLifecycleCoordinator(env RuntimeEnvironment, scope RuntimeScope, ipcServer ports.IPCServerPort, router Router, opts daemonServeOptions) daemonLifecycleCoordinator {
	return daemonLifecycleCoordinator{env: env, scope: scope, ipcServer: ipcServer, router: router, opts: opts}
}

func (c daemonLifecycleCoordinator) run(ctx context.Context) error {
	if c.opts.ensureStartup {
		if err := ensureRestoredAndCapturedOnStartup(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon ensure failed: %v\n", err)
			return err
		}
	}

	cfg := loadSidebarConfig(ctx)
	if err := resetTransientHeatStateOnStartup(ctx); err != nil {
		return err
	}

	daemonCtx, stopDaemon := context.WithCancel(ctx)
	defer stopDaemon()

	metadata := newDaemonMetadataLifecycle(daemonCtx, c.env)

	pendingFullCaptureAt, err := c.initializeCapture(daemonCtx, cfg, metadata)
	if err != nil {
		return err
	}

	var colorSchemeWG sync.WaitGroup
	colorSchemeWG.Go(func() {
		if err := NewColorSchemeServiceWithEnvironment(c.env).Run(daemonCtx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: color scheme watcher stopped: %v\n", err)
		}
	})

	runtimeEvents := make(chan ports.Request, 128)
	var eventWG sync.WaitGroup
	c.startRuntimeEventProcessor(daemonCtx, runtimeEvents, &eventWG)

	var ipcWG sync.WaitGroup
	c.startIPC(daemonCtx, metadata.reconcile, runtimeEvents, &ipcWG)

	err = c.runRefreshLoop(daemonCtx, pendingFullCaptureAt, metadata)
	stopDaemon()
	ipcWG.Wait()
	eventWG.Wait()
	metadata.wait()
	colorSchemeWG.Wait()
	return err
}

func (c daemonLifecycleCoordinator) initializeCapture(ctx context.Context, cfg ports.ConfigSnapshot, metadata *daemonMetadataLifecycle) (time.Time, error) {
	if shouldSkipSidebarSessionRestoreForContinuum(ctx, cfg) {
		return time.Now().Add(time.Duration(continuumRestoreWindowSeconds(ctx, cfg)) * time.Second), nil
	}
	// The initial config capture must populate heat baselines without consuming
	// the recent-order auto-sort interval. Startup has just cleared transient heat
	// state, so sorting here would use stale LastActiveAt values and overwrite the
	// restored persisted order before real post-startup activity exists.
	// The protected variant guards against destructive overwrite of restored state
	// when only hidden/numeric sessions are visible during startup.
	captured, err := captureLiveSidebarSessionsWithConfigProtectedPreservingOrder(ctx, cfg)
	if err != nil {
		return time.Time{}, err
	}
	if !captured {
		return time.Now().Add(sidebarRefreshIntervalFromConfig(cfg)), nil
	}
	metadata.start(cfg)
	return time.Time{}, nil
}

func (c daemonLifecycleCoordinator) startRuntimeEventProcessor(ctx context.Context, runtimeEvents <-chan ports.Request, eventWG *sync.WaitGroup) {
	if c.router == nil {
		return
	}
	eventWG.Go(func() {
		if err := runDaemonRuntimeEventProcessor(ctx, daemonRuntimeEventProcessorOptions{
			router: c.router,
			events: runtimeEvents,
			ready:  runtimeEventsReadyAfterRestore,
			stdout: io.Discard,
			stderr: os.Stderr,
		}); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: runtime event processor failed: %v\n", err)
		}
	})
}

func (c daemonLifecycleCoordinator) startIPC(ctx context.Context, metadataReconcile chan<- struct{}, runtimeEvents chan<- ports.Request, ipcWG *sync.WaitGroup) {
	if c.ipcServer == nil || c.router == nil {
		return
	}
	ipcWG.Go(func() {
		handler := daemonIPCHandler{router: c.router, stdout: io.Discard, stderr: os.Stderr, mu: &sync.Mutex{}, metadataReconcile: metadataReconcile, runtimeEvents: runtimeEvents, expectedScope: c.scope}
		if err := c.ipcServer.Serve(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: ipc server failed: %v\n", err)
		}
	})
}

func (c daemonLifecycleCoordinator) runRefreshLoop(ctx context.Context, pendingFullCaptureAt time.Time, metadata *daemonMetadataLifecycle) error {
	for {
		cfg := loadSidebarConfig(ctx)
		if pendingFullCaptureAt.IsZero() {
			metadata.start(cfg)
		}
		interval := daemonNextRefreshInterval(cfg, pendingFullCaptureAt)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimerBestEffort(timer)
			return nil
		case <-timer.C:
		}
		if current, err := runtimeScopeStillCurrent(ctx, c.scope); err != nil {
			return err
		} else if !current {
			return fmt.Errorf("daemon tmux server identity is stale")
		}
		if !pendingFullCaptureAt.IsZero() && !time.Now().Before(pendingFullCaptureAt) {
			pendingFullCaptureAt = runPendingFullCapture(ctx, cfg, metadata)
			continue
		}
		if captured, err := captureLiveSidebarHeat(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
		} else if captured {
			refreshAllSidebarPanesBestEffort(ctx)
		}
	}
}

func daemonNextRefreshInterval(cfg ports.ConfigSnapshot, pendingFullCaptureAt time.Time) time.Duration {
	interval := sidebarRefreshIntervalFromConfig(cfg)
	if pendingFullCaptureAt.IsZero() {
		return interval
	}
	untilCapture := time.Until(pendingFullCaptureAt)
	if untilCapture < interval {
		return max(untilCapture, 0)
	}
	return interval
}

func runPendingFullCapture(ctx context.Context, cfg ports.ConfigSnapshot, metadata *daemonMetadataLifecycle) time.Time {
	captured, err := captureLiveSidebarSessionsWithConfigProtectedPreservingOrder(ctx, cfg)
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
		return time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
	}
	if !captured {
		return time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
	}
	refreshAllSidebarPanesBestEffort(ctx)
	metadata.start(cfg)
	return time.Time{}
}

type daemonMetadataLifecycle struct {
	ctx           context.Context
	env           RuntimeEnvironment
	reconcile     chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	started       bool
	lastFailureAt time.Time
}

func newDaemonMetadataLifecycle(ctx context.Context, env RuntimeEnvironment) *daemonMetadataLifecycle {
	return &daemonMetadataLifecycle{ctx: ctx, env: env, reconcile: make(chan struct{}, 1)}
}

func (m *daemonMetadataLifecycle) start(cfg ports.ConfigSnapshot) {
	if !cfg.MetadataSublineEnabled {
		return
	}
	m.mu.Lock()
	if m.started || metadataWatcherRestartInCooldown(time.Now(), m.lastFailureAt) {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	m.wg.Go(func() {
		service := NewMetadataServiceWithEnvironment(m.env)
		service.ReconcileRequests = m.reconcile
		err := service.Run(m.ctx, cfg)
		failed := err != nil && !errors.Is(err, context.Canceled)
		m.mu.Lock()
		m.started = false
		if failed {
			m.lastFailureAt = time.Now()
		} else {
			m.lastFailureAt = time.Time{}
		}
		m.mu.Unlock()
		if failed {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watcher stopped: %v\n", err)
		}
	})
}

func (m *daemonMetadataLifecycle) wait() {
	m.wg.Wait()
}
