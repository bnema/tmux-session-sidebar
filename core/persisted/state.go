package persisted

import (
	"slices"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

// IsMeaningful reports whether persisted state contains user/session data worth
// protecting or migrating. Startup-only placeholders such as __startup and
// numeric sessions do not count as meaningful.
func IsMeaningful(state ports.PersistedState) bool {
	if hasPersistableSessionKey(state.Sessions) {
		return true
	}
	if hasPersistableSessionName(state.SessionOrder) {
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
