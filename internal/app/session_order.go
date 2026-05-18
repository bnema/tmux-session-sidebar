package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
)

func applySessionOrder(live []string, order []string) []string {
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}
	used := make(map[string]bool, len(live))
	ordered := make([]string, 0, len(live))
	for _, name := range order {
		if liveSet[name] && !used[name] {
			ordered = append(ordered, name)
			used[name] = true
		}
	}
	for _, name := range live {
		if !used[name] {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

func moveSessionOrder(live []string, order []string, session string, delta int) []string {
	ordered := applySessionOrder(live, order)
	index := -1
	for i, name := range ordered {
		if name == session {
			index = i
			break
		}
	}
	if index < 0 {
		return ordered
	}
	target := min(max(index+delta, 0), len(ordered)-1)
	if target == index {
		return ordered
	}
	ordered[index], ordered[target] = ordered[target], ordered[index]
	return ordered
}

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
	state.SessionOrder = moveSessionOrder(live, state.SessionOrder, session, delta)
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
