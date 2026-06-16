package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestSessionOrderStoreUsesScopedServerDirInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	defer ResetRuntimeScopeForTest()
	SetRuntimeScope(RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t321\n"}))

	store := sessionOrderStore()
	state, err := store.Load(context.Background(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.SessionOrder = []string{"alpha"}
	if err := store.Save(context.Background(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	scopedPath := filepath.Join(stateHome, "tmux-session-sidebar", "servers")
	legacyPath := filepath.Join(stateHome, "tmux-session-sidebar", "tmux.json")
	if matches, _ := filepath.Glob(filepath.Join(scopedPath, "*", "tmux.json")); len(matches) != 1 {
		t.Fatalf("scoped tmux.json matches = %#v, want one", matches)
	}
	if fileExists(legacyPath) {
		t.Fatalf("legacy tmux.json exists at %q; want scoped isolation", legacyPath)
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
