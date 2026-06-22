package persisted

import (
	"encoding/json"
	"slices"

	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

// EmptyState returns a PersistedState with map and slice fields initialized the
// same way persisted-state loads do.
func EmptyState() ports.PersistedState {
	state := ports.PersistedState{}
	InitializeState(&state)
	return state
}

// DecodeState unmarshals persisted state JSON and initializes map and slice
// fields that callers expect to be usable after loading.
func DecodeState(data []byte) (ports.PersistedState, error) {
	if len(data) == 0 {
		return EmptyState(), nil
	}
	var state ports.PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return ports.PersistedState{}, err
	}
	InitializeState(&state)
	return state, nil
}

// InitializeState normalizes nil map and slice fields after decoding persisted
// state from disk.
func InitializeState(state *ports.PersistedState) {
	if state.Sessions == nil {
		state.Sessions = map[string]ports.SessionMetadata{}
	}
	if state.SessionOrder == nil {
		state.SessionOrder = []string{}
	}
	if state.Clients == nil {
		state.Clients = map[string][]byte{}
	}
	if state.Heat == nil {
		state.Heat = map[string][]byte{}
	}
}

// IsMeaningful reports whether persisted state contains user/session data worth
// protecting or migrating. Startup-only placeholders such as __startup and
// numeric sessions do not count as meaningful. All session-name-bearing
// fields that protected captures may prune must participate in this check.
func IsMeaningful(state ports.PersistedState) bool {
	if hasPersistableSessionKey(state.Sessions) {
		return true
	}
	if hasPersistableSessionName(state.SessionOrder) {
		return true
	}
	if hasPersistableSessionName(state.PinnedSessions) {
		return true
	}
	if hasPersistableStringKey(state.PinColors) {
		return true
	}
	return hasPersistableSidebarLayoutRef(state.SidebarLayout)
}

func hasPersistableSessionKey(values map[string]ports.SessionMetadata) bool {
	for name := range values {
		if sessions.IsPersistableName(name) {
			return true
		}
	}
	return false
}

func hasPersistableSessionName(values []string) bool {
	return slices.ContainsFunc(values, sessions.IsPersistableName)
}

func hasPersistableStringKey(values map[string]string) bool {
	for name := range values {
		if sessions.IsPersistableName(name) {
			return true
		}
	}
	return false
}

func hasPersistableSidebarLayoutRef(layout *ports.SidebarLayout) bool {
	if layout == nil {
		return false
	}
	for _, item := range layout.Items {
		if item.Category == nil {
			continue
		}
		for _, ref := range item.Category.Sessions {
			if sessions.IsPersistableName(ref.Name) {
				return true
			}
		}
	}
	return false
}
