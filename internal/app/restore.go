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
	defer func() { _ = lock.Release() }()

	service := runtimeServiceWithStore(store)
	home, _ := os.UserHomeDir()
	report := service.RestorePersistedSessions(ctx, "tmux", home)
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
	defer func() { _ = lock.Release() }()

	return runtimeServiceWithStore(store).CaptureLiveSessions(ctx, "tmux")
}
