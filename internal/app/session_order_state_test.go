package app

import (
	"testing"

	sidebarlayout "github.com/bnema/tmux-session-sidebar/internal/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestRenameSessionStateUpdatesSidebarLayoutRefs(t *testing.T) {
	state := ports.PersistedState{
		Sessions:     map[string]ports.SessionMetadata{"old": {Kind: "project"}},
		SessionOrder: []string{"old"},
		SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
			{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "old"}}}},
		}},
	}

	renameSessionState(&state, "old", "new")

	if _, ok := state.Sessions["new"]; !ok {
		t.Fatalf("renamed session metadata missing: %#v", state.Sessions)
	}
	if got := state.SidebarLayout.Items[0].Category.Sessions[0].Name; got != "new" {
		t.Fatalf("layout session ref = %q, want new", got)
	}
}

func TestRemoveSessionStateRemovesSidebarLayoutRefs(t *testing.T) {
	state := ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{"old": {Kind: "project"}},
		SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
			{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "old"}, {Name: "keep"}}}},
		}},
	}

	removeSessionState(&state, "old")

	refs := state.SidebarLayout.Items[0].Category.Sessions
	if len(refs) != 1 || refs[0].Name != "keep" {
		t.Fatalf("layout refs after remove = %#v, want keep only", refs)
	}
}
