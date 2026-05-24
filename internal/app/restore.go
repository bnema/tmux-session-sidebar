package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/locker"
	adapterlogger "github.com/bnema/tmux-session-sidebar/adapters/logger"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func ensureRestoredAndCaptured(ctx context.Context) error {
	return ensureRestoredAndCapturedWithOptions(ctx, false)
}

func ensureRestoredAndCapturedAndRefresh(ctx context.Context) error {
	if err := ensureRestoredAndCaptured(ctx); err != nil {
		return err
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
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			if err == nil {
				err = fmt.Errorf("empty user home directory")
			}
			return fmt.Errorf("get user home directory: %w", err)
		}
		report := service.RestorePersistedSessions(ctx, "tmux", home)
		for name, restoreErr := range report.SystemFailures {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed (system): %v\n", name, restoreErr)
		}
		for name, restoreErr := range report.Failed {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed (session): %v\n", name, restoreErr)
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

func serveSidebarDaemon(ctx context.Context) error {
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
	// captureLiveSidebarSessionsWithConfig must succeed during daemon startup so the
	// initial session snapshot is available; later captureLiveSidebarHeat ticks only log
	// failures because stale heat/attention data is less critical than bootstrapping.
	if err := captureLiveSidebarSessionsWithConfig(ctx, cfg); err != nil {
		return err
	}

	for {
		cfg = loadSidebarConfig(ctx)
		timer := time.NewTimer(sidebarRefreshIntervalFromConfig(cfg))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
		if err := captureLiveSidebarHeat(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: daemon capture failed: %v\n", err)
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
	return 5 * time.Second
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
