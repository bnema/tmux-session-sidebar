package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestSessionOrderStoreUsesDurableStateDirInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	defer ResetRuntimeScopeForTest()
	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t321\n"})
	SetRuntimeScope(scope)

	store := sessionOrderStore()
	state, err := store.Load(context.Background(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.SessionOrder = []string{"alpha"}
	if err := store.Save(context.Background(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	statePath := filepath.Join(stateHome, "tmux-session-sidebar", "state")
	serverPath := filepath.Join(stateHome, "tmux-session-sidebar", "servers")
	legacyPath := filepath.Join(stateHome, "tmux-session-sidebar", "tmux.json")
	matches, err := filepath.Glob(filepath.Join(statePath, "*", "tmux.json"))
	if err != nil {
		t.Fatalf("glob durable state: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("durable tmux.json matches = %#v, want one", matches)
	}
	matches, err = filepath.Glob(filepath.Join(serverPath, "*", "tmux.json"))
	if err != nil {
		t.Fatalf("glob volatile servers: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("volatile server tmux.json matches = %#v, want none", matches)
	}
	if fileExists(legacyPath) {
		t.Fatalf("legacy tmux.json exists at %q; want scoped isolation", legacyPath)
	}
	if store.Dir() != scope.StateDir {
		t.Fatalf("store dir = %q, want state dir %q", store.Dir(), scope.StateDir)
	}
}

func TestSidebarStateSurvivesTmuxPIDChangeForSameSocket(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	defer ResetRuntimeScopeForTest()

	oldScope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t111\n"})
	newScope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t222\n"})
	if oldScope.Dir == newScope.Dir {
		t.Fatalf("runtime dirs should remain PID-scoped: old=%q, new=%q", oldScope.Dir, newScope.Dir)
	}
	if oldScope.StateDir != newScope.StateDir {
		t.Fatalf("state dirs should be stable across PID changes: %q vs %q", oldScope.StateDir, newScope.StateDir)
	}

	SetRuntimeScope(oldScope)
	if err := saveLoadedSidebarState(ctx, sessionOrderStore(), ports.PersistedState{PinColors: map[string]string{"alpha": "#38bdf8"}}); err != nil {
		t.Fatalf("save old PID state: %v", err)
	}

	SetRuntimeScope(newScope)
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("load new PID state: %v", err)
	}
	if got := state.PinColors["alpha"]; got != "#38bdf8" {
		t.Fatalf("PinColors[alpha] after PID change = %q, want #38bdf8", got)
	}
}

func TestSidebarStateIsIsolatedPerTmuxServerAndLegacyFallback(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	defer ResetRuntimeScopeForTest()

	scopeA := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux-a/default\t111\n"})
	scopeB := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux-b/default\t222\n"})
	SetRuntimeScope(scopeA)
	if err := saveLoadedSidebarState(ctx, sessionOrderStore(), ports.PersistedState{SessionOrder: []string{"alpha"}}); err != nil {
		t.Fatalf("save server A: %v", err)
	}
	SetRuntimeScope(scopeB)
	if err := saveLoadedSidebarState(ctx, sessionOrderStore(), ports.PersistedState{SessionOrder: []string{"beta"}}); err != nil {
		t.Fatalf("save server B: %v", err)
	}

	SetRuntimeScope(scopeA)
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("load server A: %v", err)
	}
	if got := state.SessionOrder; len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("server A order = %#v, want [alpha]", got)
	}
	SetRuntimeScope(scopeB)
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("load server B: %v", err)
	}
	if got := state.SessionOrder; len(got) != 1 || got[0] != "beta" {
		t.Fatalf("server B order = %#v, want [beta]", got)
	}

	ResetRuntimeScopeForTest()
	t.Setenv("TMUX", "")
	if err := saveLoadedSidebarState(ctx, sessionOrderStore(), ports.PersistedState{SessionOrder: []string{"legacy"}}); err != nil {
		t.Fatalf("save legacy: %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if got := state.SessionOrder; len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("legacy order = %#v, want [legacy]", got)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
