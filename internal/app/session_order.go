package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/adapters/locker"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func loadSessionOrder(ctx context.Context) []string {
	state, err := loadSidebarState(ctx)
	if err != nil {
		return nil
	}
	return state.SessionOrder
}

func loadSidebarState(ctx context.Context) (ports.PersistedState, error) {
	return sessionOrderStore().Load(ctx, "tmux")
}

func saveMovedSessionOrder(ctx context.Context, live []string, session string, delta int) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.SessionOrder = sessions.MoveOrder(live, state.SessionOrder, session, delta)
	})
}

func saveShowNumericSessions(ctx context.Context, show bool) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{ShowNumericSessions: show}
	})
}

func updateSidebarState(ctx context.Context, update func(*ports.PersistedState)) error {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		return err
	}
	update(&state)
	return store.Save(ctx, "tmux", state)
}

func sessionOrderStore() storefs.Store {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = filepath.Join(home, ".local", "state")
		} else {
			base = os.TempDir()
		}
	}
	return storefs.New(filepath.Join(base, "tmux-session-sidebar"))
}
