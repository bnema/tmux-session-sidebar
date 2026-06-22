package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	coreruntime "github.com/bnema/tmux-session-sidebar/internal/core/runtime"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

const maxSidebarLogBytes = 1024 * 1024

func ensureRestoredAndCaptured(ctx context.Context) error {
	return ensureRestoredAndCapturedWithOptions(ctx, false)
}

func ensureRestoredAndCapturedAndRefresh(ctx context.Context, client string, session string, sidebar ports.SidebarPort) error {
	if err := ensureRestoredAndCaptured(ctx); err != nil {
		return err
	}
	if !isInternalHookSession(session) {
		if err := adoptPersistedOpenSidebar(ctx, client, sidebar); err != nil {
			return err
		}
	}
	refreshAllSidebarPanesBestEffort(ctx)
	return nil
}

func ensureRestoredAndCapturedOnStartup(ctx context.Context) error {
	return ensureRestoredAndCapturedWithOptions(ctx, true)
}

func ensureRestoredAndCapturedWithOptions(ctx context.Context, resetTransientHeat bool) error {
	cfg := loadSidebarConfig(ctx)
	return withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		service := runtimeServiceWithStore(store)
		if resetTransientHeat {
			if err := service.ResetTransientHeatState(ctx, "tmux"); err != nil {
				return err
			}
		}
		skipRestore := shouldSkipSidebarSessionRestoreForContinuum(ctx, cfg)
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			if err == nil {
				err = fmt.Errorf("empty user home directory")
			}
			return fmt.Errorf("get user home directory: %w", err)
		}
		if !skipRestore {
			report := service.RestorePersistedSessions(ctx, "tmux", home)
			for name, restoreErr := range report.SystemFailures {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed (system): %v\n", name, restoreErr)
			}
			for name, restoreErr := range report.Failed {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed (session): %v\n", name, restoreErr)
			}
			if err := service.ResetTransientHeatStateForSessions(ctx, "tmux", report.Restored); err != nil {
				return err
			}
		}
		if skipRestore {
			// shouldSkipSidebarSessionRestoreForContinuum means Continuum/Resurrect
			// owns startup restore right now, so this service intentionally skips both
			// RestorePersistedSessions and the first CaptureLiveSessions; the daemon
			// performs that live capture after its pending Continuum window expires.
			return nil
		}
		captured, err := service.CaptureLiveSessionsProtected(ctx, "tmux")
		if err != nil {
			return err
		}
		if !captured {
			return nil
		}
		return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
	})
}

func resetTransientHeatStateOnStartup(ctx context.Context) error {
	return withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		return runtimeServiceWithStore(store).ResetTransientHeatState(ctx, "tmux")
	})
}

func captureLiveSidebarSessions(ctx context.Context) error {
	cfg := loadSidebarConfig(ctx)
	return withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		service := runtimeServiceWithStore(store)
		if err := service.CaptureLiveSessions(ctx, "tmux"); err != nil {
			return err
		}
		return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
	})
}

func captureLiveSidebarSessionsWithConfigProtected(ctx context.Context, cfg ports.ConfigSnapshot) (bool, error) {
	captured := false
	err := withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		return withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
			service := runtimeServiceWithStore(store).WithLogger(logger)
			var err error
			captured, err = service.CaptureLiveSessionsWithConfigProtected(ctx, "tmux", cfg)
			if err != nil || !captured {
				return err
			}
			return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
		})
	})
	return captured, err
}

func captureLiveSidebarHeat(ctx context.Context, cfg ports.ConfigSnapshot) (bool, error) {
	if !coreruntime.SessionHeatCaptureRequired(cfg) {
		return false, nil
	}
	err := withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		return withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
			service := runtimeServiceWithStore(store).WithLogger(logger)
			return service.CaptureSessionHeatWithConfig(ctx, "tmux", cfg)
		})
	})
	return err == nil, err
}

func bootstrapSidebarDaemon(ctx context.Context, _ io.Writer, ipcServer ports.IPCServerPort, router Router) error {
	scope := CurrentRuntimeScope()
	if err := EnsureRuntimeDirPrivate(scope.Dir); err != nil {
		return err
	}
	restoreStderr, err := redirectStderrToRotatingLog(scope.ErrorsLogPath, maxSidebarLogBytes)
	if err != nil {
		return err
	}
	defer restoreStderr()
	return serveSidebarDaemonWithOptions(ctx, ipcServer, router, daemonServeOptions{ensureStartup: true})
}

type daemonServeOptions struct {
	ensureStartup bool
}

func serveSidebarDaemon(ctx context.Context, ipcServer ports.IPCServerPort, router Router) error {
	return serveSidebarDaemonWithOptions(ctx, ipcServer, router, daemonServeOptions{})
}

func serveSidebarDaemonWithOptions(ctx context.Context, ipcServer ports.IPCServerPort, router Router, opts daemonServeOptions) error {
	scope := CurrentRuntimeScope()
	if current, err := runtimeScopeStillCurrent(ctx, scope); err != nil {
		return err
	} else if !current {
		return fmt.Errorf("daemon tmux server identity is stale")
	}
	if err := writeRuntimeScopeMetadata(scope); err != nil {
		return err
	}

	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock, err := runtimeLocker(scope.LocksDir).Acquire(acquireCtx, "tmux-sidebar-daemon")
	if err != nil {
		// A timeout/cancel here usually means another daemon already holds the lock, so
		// treat it as a no-op instead of failing startup and racing concurrent restores.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	defer releaseSidebarLock(lock)

	// Migrate tmux.json state from a prior tmux server instance that used
	// the same socket. On full tmux restart the PID changes so the current
	// scope is empty; this copies the previous meaningful state into it.
	// This runs after the daemon lock is acquired so no other daemon can
	// race us, and internally acquires the tmux-sidebar-state lock to guard
	// against concurrent startup commands writing tmux.json in this scope.
	if err := ensureRuntimeStateMigrated(ctx, scope); err != nil {
		return fmt.Errorf("runtime state migration for %s: %w", scope.Dir, err)
	}

	pidFile := scope.PIDPath
	if err := os.WriteFile(pidFile, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon serve started pid=%d\n", os.Getpid())
	defer func() {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon serve stopped pid=%d\n", os.Getpid())
		_ = os.Remove(pidFile)
	}()

	if opts.ensureStartup {
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
	metadataReconcile := make(chan struct{}, 1)
	var metadataWG sync.WaitGroup
	metadataStarted := false
	startMetadataWatcher := func(cfg ports.ConfigSnapshot) {
		if metadataStarted || !cfg.MetadataSublineEnabled {
			return
		}
		metadataStarted = true
		metadataWG.Go(func() {
			service := NewMetadataService()
			service.ReconcileRequests = metadataReconcile
			if err := service.Run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watcher stopped: %v\n", err)
			}
		})
	}
	pendingFullCaptureAt := time.Time{}
	if shouldSkipSidebarSessionRestoreForContinuum(ctx, cfg) {
		pendingFullCaptureAt = time.Now().Add(time.Duration(continuumRestoreWindowSeconds(ctx, cfg)) * time.Second)
	} else {
		// captureLiveSidebarSessionsWithConfigProtected must succeed during daemon startup so the
		// initial session snapshot is available; later captureLiveSidebarHeat ticks only log
		// failures because stale heat/attention data is less critical than bootstrapping.
		// The protected variant guards against destructive overwrite of restored state
		// when only hidden/numeric sessions are visible during startup.
		captured, err := captureLiveSidebarSessionsWithConfigProtected(ctx, cfg)
		if err != nil {
			return err
		}
		if !captured {
			pendingFullCaptureAt = time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
		} else {
			startMetadataWatcher(cfg)
		}
	}
	var ipcWG sync.WaitGroup
	if ipcServer != nil && router != nil {
		ipcWG.Go(func() {
			if err := ipcServer.Serve(ctx, daemonIPCHandler{router: router, stdout: io.Discard, stderr: os.Stderr, mu: &sync.Mutex{}, metadataReconcile: metadataReconcile, expectedScope: scope}); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: ipc server failed: %v\n", err)
			}
		})
	}

	for {
		cfg = loadSidebarConfig(ctx)
		interval := sidebarRefreshIntervalFromConfig(cfg)
		if !pendingFullCaptureAt.IsZero() {
			untilCapture := time.Until(pendingFullCaptureAt)
			if untilCapture < interval {
				interval = max(untilCapture, 0)
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimerBestEffort(timer)
			ipcWG.Wait()
			metadataWG.Wait()
			colorSchemeWG.Wait()
			return nil
		case <-timer.C:
		}
		if current, err := runtimeScopeStillCurrent(ctx, scope); err != nil {
			return err
		} else if !current {
			return fmt.Errorf("daemon tmux server identity is stale")
		}
		if !pendingFullCaptureAt.IsZero() && !time.Now().Before(pendingFullCaptureAt) {
			captured, err := captureLiveSidebarSessionsWithConfigProtected(ctx, cfg)
			if err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
				pendingFullCaptureAt = time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
			} else if !captured {
				pendingFullCaptureAt = time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
			} else {
				pendingFullCaptureAt = time.Time{}
				refreshAllSidebarPanesBestEffort(ctx)
				startMetadataWatcher(cfg)
			}
			continue
		}
		if captured, err := captureLiveSidebarHeat(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
		} else if captured {
			refreshAllSidebarPanesBestEffort(ctx)
		}
	}
}

func captureVisitedAgentAttentionIfEnabled(ctx context.Context, service interface {
	CaptureVisitedAgentAttention(context.Context, string) error
}, cfg ports.ConfigSnapshot) error {
	if !cfg.AgentAttentionEnabled {
		return nil
	}
	return service.CaptureVisitedAgentAttention(ctx, "tmux")
}

func withActivityDebugLogger(cfg ports.ConfigSnapshot, fn func(logger ports.LoggerPort) error) error {
	if !cfg.ActivityDebugLog {
		return fn(nil)
	}
	logPath := filepath.Join(RuntimeDir(), "activity.log")
	logger, closer, err := runtimeActivityLogger(logPath, maxSidebarLogBytes)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	return fn(logger)
}

func sidebarRefreshIntervalFromConfig(cfg ports.ConfigSnapshot) time.Duration {
	if cfg.HeatRefreshSeconds > 0 {
		return time.Duration(cfg.HeatRefreshSeconds) * time.Second
	}
	return time.Minute
}

func withLockedSidebarStore(ctx context.Context, fn func(scopedStateStore) error) error {
	store := sessionOrderStore()
	if err := ensureRuntimeStateMigrated(ctx, store.scope); err != nil {
		return err
	}
	lock, err := runtimeLocker(filepath.Join(store.Dir(), "locks")).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return err
	}
	defer releaseSidebarLock(lock)
	return fn(store)
}

func releaseSidebarLock(lock interface{ Release() error }) {
	if err := lock.Release(); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: release state lock failed: %v\n", err)
	}
}
