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

	"github.com/bnema/tmux-session-sidebar/adapters/locker"
	adapterlogger "github.com/bnema/tmux-session-sidebar/adapters/logger"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func ensureRestoredAndCaptured(ctx context.Context) error {
	return ensureRestoredAndCapturedWithOptions(ctx, false)
}

func ensureRestoredAndCapturedAndRefresh(ctx context.Context, client string, session string, sidebar ports.TmuxSidebarPort) error {
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
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
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
		}
		if skipRestore {
			// shouldSkipSidebarSessionRestoreForContinuum means Continuum/Resurrect
			// owns startup restore right now, so this service intentionally skips both
			// RestorePersistedSessions and the first CaptureLiveSessions; the daemon
			// performs that live capture after its pending Continuum window expires.
			return nil
		}
		if err := service.CaptureLiveSessions(ctx, "tmux"); err != nil {
			return err
		}
		return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
	})
}

func resetTransientHeatStateOnStartup(ctx context.Context) error {
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		return runtimeServiceWithStore(store).ResetTransientHeatState(ctx, "tmux")
	})
}

func captureLiveSidebarSessions(ctx context.Context) error {
	cfg := loadSidebarConfig(ctx)
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		service := runtimeServiceWithStore(store)
		if err := service.CaptureLiveSessions(ctx, "tmux"); err != nil {
			return err
		}
		return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
	})
}

func captureLiveSidebarSessionsWithConfig(ctx context.Context, cfg ports.ConfigSnapshot) error {
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		return withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
			service := runtimeServiceWithStore(store).WithLogger(logger)
			if err := service.CaptureLiveSessionsWithConfig(ctx, "tmux", cfg); err != nil {
				return err
			}
			return captureVisitedAgentAttentionIfEnabled(ctx, service, cfg)
		})
	})
}

func captureLiveSidebarHeat(ctx context.Context, cfg ports.ConfigSnapshot) error {
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		return withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
			service := runtimeServiceWithStore(store).WithLogger(logger)
			return service.CaptureSessionHeatWithConfig(ctx, "tmux", cfg)
		})
	})
}

func serveSidebarDaemon(ctx context.Context, ipcServer ports.IPCServerPort, router Router) error {
	store := sessionOrderStore()
	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(acquireCtx, "tmux-sidebar-daemon")
	if err != nil {
		// A timeout/cancel here usually means another daemon already holds the lock, so
		// treat it as a no-op instead of failing startup and racing concurrent restores.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	defer releaseSidebarLock(lock)
	pidFile := filepath.Join(store.Dir, "daemon.pid")
	if err := os.WriteFile(pidFile, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(pidFile) }()

	cfg := loadSidebarConfig(ctx)
	if err := resetTransientHeatStateOnStartup(ctx); err != nil {
		return err
	}
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
		// captureLiveSidebarSessionsWithConfig must succeed during daemon startup so the
		// initial session snapshot is available; later captureLiveSidebarHeat ticks only log
		// failures because stale heat/attention data is less critical than bootstrapping.
		if err := captureLiveSidebarSessionsWithConfig(ctx, cfg); err != nil {
			return err
		}
		startMetadataWatcher(cfg)
	}
	var ipcWG sync.WaitGroup
	if ipcServer != nil && router != nil {
		ipcWG.Go(func() {
			if err := ipcServer.Serve(ctx, daemonIPCHandler{router: router, stdout: io.Discard, stderr: os.Stderr, mu: &sync.Mutex{}, metadataReconcile: metadataReconcile}); err != nil && !errors.Is(err, context.Canceled) {
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
			if !timer.Stop() {
				<-timer.C
			}
			ipcWG.Wait()
			metadataWG.Wait()
			return nil
		case <-timer.C:
		}
		if !pendingFullCaptureAt.IsZero() && !time.Now().Before(pendingFullCaptureAt) {
			if err := captureLiveSidebarSessionsWithConfig(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
				pendingFullCaptureAt = time.Now().Add(sidebarRefreshIntervalFromConfig(cfg))
			} else {
				pendingFullCaptureAt = time.Time{}
				refreshAllSidebarPanesBestEffort(ctx)
				startMetadataWatcher(cfg)
			}
			continue
		}
		if err := captureLiveSidebarHeat(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
		} else {
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
	logPath := filepath.Join(sessionOrderStore().Dir, "activity.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return fn(adapterlogger.Logger{Out: file})
}

func sidebarRefreshIntervalFromConfig(cfg ports.ConfigSnapshot) time.Duration {
	if cfg.HeatRefreshSeconds > 0 {
		return time.Duration(cfg.HeatRefreshSeconds) * time.Second
	}
	return time.Minute
}

func withLockedSidebarStore(ctx context.Context, fn func(storefs.Store) error) error {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
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
