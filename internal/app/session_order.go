package app

import (
	"context"
	"maps"
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

func snapshotSidebarState(ctx context.Context) (ports.PersistedState, error) {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return ports.PersistedState{}, err
	}
	defer releaseSidebarLock(lock)
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		return ports.PersistedState{}, err
	}
	return clonePersistedState(state), nil
}

func restoreSidebarState(ctx context.Context, state ports.PersistedState) error {
	return updateSidebarState(ctx, func(current *ports.PersistedState) {
		*current = clonePersistedState(state)
	})
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

func saveSessionMetadata(ctx context.Context, name string, metadata ports.SessionMetadata) error {
	_, err := saveSessionMetadataWithSnapshot(ctx, name, metadata)
	return err
}

func saveSessionMetadataWithSnapshot(ctx context.Context, name string, metadata ports.SessionMetadata) (ports.PersistedState, error) {
	var previous ports.PersistedState
	if !shouldPersistSessionName(name) {
		// Persistence is intentionally skipped, but callers still need a rollback snapshot.
		state, err := snapshotSidebarState(ctx)
		return state, err
	}
	err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		previous = clonePersistedState(*state)
		if state.Sessions == nil {
			state.Sessions = map[string]ports.SessionMetadata{}
		}
		state.Sessions[name] = metadata
	})
	return previous, err
}

func persistedSessionMetadata(ctx context.Context, name string) (ports.SessionMetadata, bool) {
	state, err := loadSidebarState(ctx)
	if err != nil || state.Sessions == nil {
		return ports.SessionMetadata{}, false
	}
	metadata, ok := state.Sessions[name]
	return metadata, ok
}

func renamePersistedSession(ctx context.Context, oldName string, newName string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		if !shouldPersistSessionName(newName) {
			delete(state.Sessions, oldName)
			filtered := state.SessionOrder[:0]
			for _, existing := range state.SessionOrder {
				if existing != oldName {
					filtered = append(filtered, existing)
				}
			}
			state.SessionOrder = filtered
			return
		}
		if state.Sessions != nil {
			metadata, ok := state.Sessions[oldName]
			if ok {
				delete(state.Sessions, oldName)
				state.Sessions[newName] = metadata
			}
		}
		for i, name := range state.SessionOrder {
			if name == oldName {
				state.SessionOrder[i] = newName
				break
			}
		}
	})
}

func removePersistedSession(ctx context.Context, name string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		delete(state.Sessions, name)
		filtered := state.SessionOrder[:0]
		for _, existing := range state.SessionOrder {
			if existing != name {
				filtered = append(filtered, existing)
			}
		}
		state.SessionOrder = filtered
	})
}

func shouldPersistSessionName(name string) bool {
	return name != "" && sessions.ValidateName(name) == nil && !sessions.IsNumericName(name) && !sessions.IsHiddenName(name)
}

func liveSessionExists(ctx context.Context, name string) bool {
	views, err := loadSessionViews(ctx)
	if err != nil {
		return false
	}
	for _, view := range views {
		if view.Name == name {
			return true
		}
	}
	return false
}

func updateSidebarState(ctx context.Context, update func(*ports.PersistedState)) error {
	store := sessionOrderStore()
	lock, err := (locker.FileLocker{Dir: filepath.Join(store.Dir, "locks")}).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return err
	}
	defer releaseSidebarLock(lock)
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

func clonePersistedState(state ports.PersistedState) ports.PersistedState {
	clone := state
	if state.Sessions != nil {
		clone.Sessions = make(map[string]ports.SessionMetadata, len(state.Sessions))
		maps.Copy(clone.Sessions, state.Sessions)
	}
	if state.SessionOrder != nil {
		clone.SessionOrder = append([]string(nil), state.SessionOrder...)
	}
	if state.Sidebar != nil {
		sidebar := *state.Sidebar
		clone.Sidebar = &sidebar
	}
	if state.Clients != nil {
		clone.Clients = make(map[string][]byte, len(state.Clients))
		for key, value := range state.Clients {
			clone.Clients[key] = append([]byte(nil), value...)
		}
	}
	if state.Heat != nil {
		clone.Heat = make(map[string][]byte, len(state.Heat))
		for key, value := range state.Heat {
			clone.Heat[key] = append([]byte(nil), value...)
		}
	}
	return clone
}
