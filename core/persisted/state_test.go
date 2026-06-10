package persisted

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestIsMeaningfulIncludesPinnedSessions(t *testing.T) {
	state := ports.PersistedState{PinnedSessions: []string{"alpha"}}
	if !IsMeaningful(state) {
		t.Fatal("IsMeaningful() = false, want true for persistable pinned session")
	}
}

func TestIsMeaningfulIncludesPinColorKeys(t *testing.T) {
	state := ports.PersistedState{PinColors: map[string]string{"alpha": "red"}}
	if !IsMeaningful(state) {
		t.Fatal("IsMeaningful() = false, want true for persistable pin color key")
	}
}

func TestIsMeaningfulIgnoresNonPersistablePinsAndColors(t *testing.T) {
	state := ports.PersistedState{
		PinnedSessions: []string{"__startup", "123"},
		PinColors:      map[string]string{"__startup": "red", "123": "blue"},
	}
	if IsMeaningful(state) {
		t.Fatal("IsMeaningful() = true, want false for only hidden/numeric pins and colors")
	}
}
