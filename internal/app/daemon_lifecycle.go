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
	scope     RuntimeScope
	ipcServer ports.IPCServerPort
	router    Router
	opts      daemonServeOptions
}

func newDaemonLifecycleCoordinator(scope RuntimeScope, ipcServer ports.IPCServerPort, router Router, opts daemonServeOptions) daemonLifecycleCoordinator {
	return daemonLifecycleCoordinator{scope: scope, ipcServer: ipcServer, router: router, opts: opts}
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

	var colorSchemeWG sync.WaitGroup
	colorSchemeWG.Go(func() {
		if err := NewColorSchemeService().Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: color scheme watcher stopped: %v\n", err)
		}
	})

	metadata := newDaemonMetadataLifecycle(ctx)

	pendingFullCaptureAt, err := c.initializeCapture(ctx, cfg, metadata)
	if err != nil {
		return err
	}

	var ipcWG sync.WaitGroup
	c.startIPC(ctx, metadata.reconcile, &ipcWG)

	return c.runRefreshLoop(ctx, pendingFullCaptureAt, metadata, &ipcWG, &colorSchemeWG)
}

func (c daemonLifecycleCoordinator) initializeCapture(ctx context.Context, cfg ports.ConfigSnapshot, metadata *daemonMetadataLifecycle) (time.Time, error) {
	if shouldSkipSidebarSessionRestoreForContinuum(ctx, cfg) {
		return time.Now().Add(time.Duration(continuumRestoreWindowSeconds(ctx, cfg)) * time.Second), nil
	}
	// captureLiveSidebarSessionsWithConfigProtected must succeed during daemon startup so the
	// initial session snapshot is available; later captureLiveSidebarHeat ticks only log
	// failures because stale heat/attention data is less critical than bootstrapping.
	// The protected variant guards against destructive overwrite of restored state
	// when only hidden/numeric sessions are visible during startup.
	captured, err := captureLiveSidebarSessionsWithConfigProtected(ctx, cfg)
	if err != nil {
		return time.Time{}, err
	}
	if !captured {
		return time.Now().Add(sidebarRefreshIntervalFromConfig(cfg)), nil
	}
	metadata.start(cfg)
	return time.Time{}, nil
}

func (c daemonLifecycleCoordinator) startIPC(ctx context.Context, metadataReconcile chan<- struct{}, ipcWG *sync.WaitGroup) {
	if c.ipcServer == nil || c.router == nil {
		return
	}
	ipcWG.Go(func() {
		handler := daemonIPCHandler{router: c.router, stdout: io.Discard, stderr: os.Stderr, mu: &sync.Mutex{}, metadataReconcile: metadataReconcile, expectedScope: c.scope}
		if err := c.ipcServer.Serve(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: ipc server failed: %v\n", err)
		}
	})
}

func (c daemonLifecycleCoordinator) runRefreshLoop(ctx context.Context, pendingFullCaptureAt time.Time, metadata *daemonMetadataLifecycle, ipcWG *sync.WaitGroup, colorSchemeWG *sync.WaitGroup) error {
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
			ipcWG.Wait()
			metadata.wait()
			colorSchemeWG.Wait()
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
	captured, err := captureLiveSidebarSessionsWithConfigProtected(ctx, cfg)
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
	reconcile     chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	started       bool
	lastFailureAt time.Time
}

func newDaemonMetadataLifecycle(ctx context.Context) *daemonMetadataLifecycle {
	return &daemonMetadataLifecycle{ctx: ctx, reconcile: make(chan struct{}, 1)}
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
		service := NewMetadataService()
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
