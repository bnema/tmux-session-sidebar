package app

import (
	"context"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestSidebarStateOpeningSecondClientPreservesFirstVisibleClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()

	if err := saveSidebarVisibility(ctx, true, "client-1"); err != nil {
		t.Fatalf("save first visibility: %v", err)
	}
	if err := saveSidebarVisibility(ctx, true, "client-2"); err != nil {
		t.Fatalf("save second visibility: %v", err)
	}

	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState: %v", err)
	}
	if !state.Open {
		t.Fatal("Open = false, want aggregate open with visible clients")
	}
	if state.VisibleClients == nil || !state.VisibleClients["client-1"] || !state.VisibleClients["client-2"] {
		t.Fatalf("VisibleClients = %#v, want client-1 and client-2 true", state.VisibleClients)
	}
	if !sidebarStateAppliesToClient(state, "client-1") || !sidebarStateAppliesToClient(state, "client-2") {
		t.Fatalf("sidebarStateAppliesToClient rejected visible client, state = %#v", state)
	}
}

func TestSidebarStateClosingOneVisibleClientLeavesOtherOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()

	if err := saveSidebarVisibility(ctx, true, "client-1"); err != nil {
		t.Fatalf("save first visibility: %v", err)
	}
	if err := saveSidebarVisibility(ctx, true, "client-2"); err != nil {
		t.Fatalf("save second visibility: %v", err)
	}
	if err := saveSidebarVisibility(ctx, false, "client-1"); err != nil {
		t.Fatalf("close first visibility: %v", err)
	}

	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState: %v", err)
	}
	if !state.Open {
		t.Fatal("Open = false, want still open for remaining visible client")
	}
	if state.VisibleClients["client-1"] {
		t.Fatalf("client-1 remains visible after scoped close: %#v", state.VisibleClients)
	}
	if !state.VisibleClients["client-2"] || !sidebarStateAppliesToClient(state, "client-2") {
		t.Fatalf("client-2 not visible after scoped close: %#v", state.VisibleClients)
	}
}

func TestSidebarStateClosingOwnerChoosesDeterministicNextOwner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{
			Open:           true,
			OwnerClient:    "client-b",
			VisibleClients: map[string]bool{"client-z": true, "client-a": true, "client-b": true},
		}
	}); err != nil {
		t.Fatalf("seed sidebar state: %v", err)
	}
	if err := saveSidebarVisibility(ctx, false, "client-b"); err != nil {
		t.Fatalf("close owner visibility: %v", err)
	}

	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState: %v", err)
	}
	if state.OwnerClient != "client-a" {
		t.Fatalf("OwnerClient = %q, want deterministic lexicographic successor client-a", state.OwnerClient)
	}
}

func TestSidebarStateLegacyOwnerOpenAppliesOnlyToOwnerClient(t *testing.T) {
	state := ports.SidebarState{Open: true, OwnerClient: "client-1"}

	if !sidebarStateAppliesToClient(state, "client-1") {
		t.Fatal("legacy owner open did not apply to owner client")
	}
	if sidebarStateAppliesToClient(state, "client-2") {
		t.Fatal("legacy owner open applied to non-owner client-2")
	}
}

func TestSidebarStateLegacyGlobalOpenDoesNotApplyToArbitraryClient(t *testing.T) {
	state := ports.SidebarState{Open: true, OwnerClient: ""}

	if sidebarStateAppliesToClient(state, "client-2") {
		t.Fatal("legacy global open applied to arbitrary non-empty client-2")
	}
}

func TestSidebarStateVisibleClientsAreDeepCloned(t *testing.T) {
	original := ports.PersistedState{Sidebar: &ports.SidebarState{Open: true, VisibleClients: map[string]bool{"client-1": true}}}

	portsClone := ports.ClonePersistedState(original)
	portsClone.Sidebar.VisibleClients["client-1"] = false
	portsClone.Sidebar.VisibleClients["client-2"] = true
	if !original.Sidebar.VisibleClients["client-1"] || original.Sidebar.VisibleClients["client-2"] {
		t.Fatalf("ports.ClonePersistedState shared VisibleClients map with original: original=%#v clone=%#v", original.Sidebar.VisibleClients, portsClone.Sidebar.VisibleClients)
	}

	appClone := clonePersistedState(original)
	appClone.Sidebar.VisibleClients["client-1"] = false
	appClone.Sidebar.VisibleClients["client-2"] = true
	if !original.Sidebar.VisibleClients["client-1"] || original.Sidebar.VisibleClients["client-2"] {
		t.Fatalf("app clonePersistedState shared VisibleClients map with original: original=%#v clone=%#v", original.Sidebar.VisibleClients, appClone.Sidebar.VisibleClients)
	}
}
