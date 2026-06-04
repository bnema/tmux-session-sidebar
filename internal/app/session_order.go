package app

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func loadSidebarState(ctx context.Context) (ports.PersistedState, error) {
	return sessionOrderStore().Load(ctx, "tmux")
}

func snapshotSidebarState(ctx context.Context) (ports.PersistedState, error) {
	var snapshot ports.PersistedState
	err := withLoadedSidebarState(ctx, func(_ storefs.Store, state *ports.PersistedState) error {
		snapshot = clonePersistedState(*state)
		return nil
	})
	return snapshot, err
}

func saveMovedSessionOrder(ctx context.Context, live []string, session string, delta int) error {
	return saveMovedVisibleSessionOrder(ctx, live, session, delta, false)
}

func saveMovedVisibleSessionOrder(ctx context.Context, live []string, session string, delta int, showNumeric bool) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.SessionOrder = sessions.MoveVisibleOrder(live, state.SessionOrder, session, delta, showNumeric)
	})
}

func saveShowNumericSessions(ctx context.Context, show bool) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		ensurePersistedSidebarState(state).ShowNumericSessions = show
	})
}

// saveToggledPinnedSession preserves an existing pin color when toggling a
// session on. New pin colors are assigned through savePinnedSessionColor; the
// color is removed only when this toggle unpins the session.
func saveToggledPinnedSession(ctx context.Context, live []string, session string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		var pinned bool
		state.PinnedSessions, pinned = sessions.TogglePinned(sessions.ReconcilePinned(state.PinnedSessions, live), session)
		if !pinned {
			delete(state.PinColors, session)
		}
		state.SessionOrder = sessions.ApplyOrder(live, state.SessionOrder)
	})
}

func savePinnedSessionColor(ctx context.Context, live []string, session string, color string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.PinnedSessions = sessions.ReconcilePinned(state.PinnedSessions, live)
		if !slices.Contains(state.PinnedSessions, session) {
			if !slices.Contains(live, session) {
				state.SessionOrder = sessions.ApplyOrder(live, state.SessionOrder)
				return
			}
			state.PinnedSessions = append(state.PinnedSessions, session)
		}
		if state.PinColors == nil {
			state.PinColors = map[string]string{}
		}
		state.PinColors[session] = color
		state.SessionOrder = sessions.ApplyOrder(live, state.SessionOrder)
	})
}

func saveSessionMetadata(ctx context.Context, name string, metadata ports.SessionMetadata) error {
	_, err := saveSessionMetadataWithSnapshot(ctx, name, metadata)
	return err
}

func saveSessionMetadataWithSnapshot(ctx context.Context, name string, metadata ports.SessionMetadata) (ports.PersistedState, error) {
	if !shouldPersistSessionName(name) {
		// Persistence is intentionally skipped, but callers still need a rollback snapshot.
		state, err := snapshotSidebarState(ctx)
		return state, err
	}
	return updateSidebarStateWithSnapshot(ctx, func(state *ports.PersistedState) {
		saveSessionMetadataState(state, name, metadata)
	})
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
		renameSessionState(state, oldName, newName)
	})
}

func renameSessionState(state *ports.PersistedState, oldName string, newName string) {
	if !shouldPersistSessionName(newName) {
		removeSessionState(state, oldName)
		return
	}
	updateSidebarLayoutSessionRef(state.SidebarLayout, oldName, newName)
	if state.Sessions != nil {
		metadata, ok := state.Sessions[oldName]
		if ok {
			delete(state.Sessions, oldName)
			state.Sessions[newName] = metadata
		}
	}
	if state.Metadata != nil {
		metadata, ok := state.Metadata[oldName]
		if ok {
			delete(state.Metadata, oldName)
			state.Metadata[newName] = metadata
		}
	}
	for i, name := range state.SessionOrder {
		if name == oldName {
			state.SessionOrder[i] = newName
			break
		}
	}
	for i, name := range state.PinnedSessions {
		if name == oldName {
			state.PinnedSessions[i] = newName
			break
		}
	}
	if color, ok := state.PinColors[oldName]; ok {
		delete(state.PinColors, oldName)
		state.PinColors[newName] = color
	}
}

func removePersistedSession(ctx context.Context, name string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		removeSessionState(state, name)
	})
}

func shouldPersistSessionName(name string) bool {
	return sessions.IsPersistableName(name)
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
	_, err := updateSidebarStateWithSnapshot(ctx, func(state *ports.PersistedState) {
		update(state)
	})
	return err
}

func updateSidebarStateWithSnapshot(ctx context.Context, update func(*ports.PersistedState)) (ports.PersistedState, error) {
	var previous ports.PersistedState
	err := withLoadedSidebarState(ctx, func(store storefs.Store, state *ports.PersistedState) error {
		previous = clonePersistedState(*state)
		update(state)
		return saveLoadedSidebarState(ctx, store, *state)
	})
	return previous, err
}

func StateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = filepath.Join(home, ".local", "state")
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "tmux-session-sidebar")
}

func sessionOrderStore() storefs.Store {
	return storefs.New(StateDir())
}

func withLoadedSidebarState(ctx context.Context, fn func(store storefs.Store, state *ports.PersistedState) error) error {
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		state, err := store.Load(ctx, "tmux")
		if err != nil {
			return err
		}
		return fn(store, &state)
	})
}

func saveLoadedSidebarState(ctx context.Context, store storefs.Store, state ports.PersistedState) error {
	return store.Save(ctx, "tmux", state)
}

func saveSessionMetadataState(state *ports.PersistedState, name string, metadata ports.SessionMetadata) {
	if state.Sessions == nil {
		state.Sessions = map[string]ports.SessionMetadata{}
	}
	state.Sessions[name] = metadata
}

func removeSessionState(state *ports.PersistedState, name string) {
	delete(state.Sessions, name)
	filtered := state.SessionOrder[:0]
	for _, existing := range state.SessionOrder {
		if existing != name {
			filtered = append(filtered, existing)
		}
	}
	state.SessionOrder = filtered
	pinned := state.PinnedSessions[:0]
	for _, existing := range state.PinnedSessions {
		if existing != name {
			pinned = append(pinned, existing)
		}
	}
	state.PinnedSessions = pinned
	delete(state.PinColors, name)
	delete(state.Metadata, name)
	removeSidebarLayoutSessionRef(state.SidebarLayout, name)
}

func updateSidebarLayoutSessionRef(layout *ports.SidebarLayout, oldName string, newName string) {
	if layout == nil {
		return
	}
	for itemIndex := range layout.Items {
		category := layout.Items[itemIndex].Category
		if category == nil {
			continue
		}
		for refIndex := range category.Sessions {
			if category.Sessions[refIndex].Name == oldName {
				category.Sessions[refIndex].Name = newName
			}
		}
	}
}

func removeSidebarLayoutSessionRef(layout *ports.SidebarLayout, name string) {
	if layout == nil {
		return
	}
	for itemIndex := range layout.Items {
		category := layout.Items[itemIndex].Category
		if category == nil {
			continue
		}
		filtered := category.Sessions[:0]
		for _, ref := range category.Sessions {
			if ref.Name != name {
				filtered = append(filtered, ref)
			}
		}
		category.Sessions = filtered
	}
}

func clonePersistedState(state ports.PersistedState) ports.PersistedState {
	clone := state
	if state.Sessions != nil {
		clone.Sessions = make(map[string]ports.SessionMetadata, len(state.Sessions))
		maps.Copy(clone.Sessions, state.Sessions)
	}
	if state.Metadata != nil {
		clone.Metadata = maps.Clone(state.Metadata)
	}
	if state.SessionOrder != nil {
		clone.SessionOrder = append([]string(nil), state.SessionOrder...)
	}
	if state.PinnedSessions != nil {
		clone.PinnedSessions = append([]string(nil), state.PinnedSessions...)
	}
	if state.PinColors != nil {
		clone.PinColors = maps.Clone(state.PinColors)
	}
	if state.Sidebar != nil {
		sidebar := *state.Sidebar
		clone.Sidebar = &sidebar
	}
	if state.SidebarLayout != nil {
		clone.SidebarLayout = clonePersistedSidebarLayout(state.SidebarLayout)
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
	if state.AgentAttention != nil {
		clone.AgentAttention = make(map[string][]byte, len(state.AgentAttention))
		for key, value := range state.AgentAttention {
			clone.AgentAttention[key] = append([]byte(nil), value...)
		}
	}
	return clone
}

func clonePersistedSidebarLayout(layout *ports.SidebarLayout) *ports.SidebarLayout {
	if layout == nil {
		return nil
	}
	clone := &ports.SidebarLayout{Items: make([]ports.SidebarLayoutItem, len(layout.Items))}
	for i, item := range layout.Items {
		clone.Items[i] = item
		if item.Category != nil {
			category := *item.Category
			category.Sessions = append([]ports.SidebarLayoutSessionRef(nil), item.Category.Sessions...)
			clone.Items[i].Category = &category
		}
		if item.Separator != nil {
			separator := *item.Separator
			clone.Items[i].Separator = &separator
		}
		if item.Spacer != nil {
			spacer := *item.Spacer
			clone.Items[i].Spacer = &spacer
		}
	}
	return clone
}
