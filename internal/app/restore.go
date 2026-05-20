package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/adapters/locker"
)

func ensureRestoredAndCaptured(ctx context.Context) error {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return err
	}
	defer releaseSidebarLock(lock)

	service := runtimeServiceWithStore(store)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if err == nil {
			err = fmt.Errorf("empty user home directory")
		}
		return fmt.Errorf("get user home directory: %w", err)
	}
	report := service.RestorePersistedSessions(ctx, "tmux", home)
	for name, restoreErr := range report.SystemFailures {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed: %v\n", name, restoreErr)
	}
	for name, restoreErr := range report.Failed {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: restore %s failed: %v\n", name, restoreErr)
	}
	return service.CaptureLiveSessions(ctx, "tmux")
}

func captureLiveSidebarSessions(ctx context.Context) error {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return err
	}
	defer releaseSidebarLock(lock)

	return runtimeServiceWithStore(store).CaptureLiveSessions(ctx, "tmux")
}

func releaseSidebarLock(lock interface{ Release() error }) {
	if err := lock.Release(); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: release state lock failed: %v\n", err)
	}
}
