package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func loadSessionOrder(ctx context.Context) []string {
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		return nil
	}
	return state.SessionOrder
}

func saveMovedSessionOrder(ctx context.Context, live []string, session string, delta int) error {
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		return err
	}
	state.SessionOrder = sessions.MoveOrder(live, state.SessionOrder, session, delta)
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
