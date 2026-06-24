package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type environmentTestStore struct{ path string }

func (s environmentTestStore) Load(context.Context, string) (ports.PersistedState, error) {
	return ports.PersistedState{}, nil
}

func (s environmentTestStore) Save(context.Context, string, ports.PersistedState) error { return nil }

func runtimeScopeOverrideForTest() (RuntimeScope, bool) {
	runtimeScopeOverride.mu.RLock()
	defer runtimeScopeOverride.mu.RUnlock()
	return runtimeScopeOverride.scope, runtimeScopeOverride.set
}

func restoreRuntimeScopeOverrideForTest(scope RuntimeScope, set bool) {
	runtimeScopeOverride.mu.Lock()
	defer runtimeScopeOverride.mu.Unlock()
	runtimeScopeOverride.scope = scope
	runtimeScopeOverride.set = set
}

func TestRuntimeEnvironmentConstructorsDoNotBleedThroughPackageGlobals(t *testing.T) {
	previousDeps := runtimeDependencies()
	previousScope, previousScopeSet := runtimeScopeOverrideForTest()
	t.Cleanup(func() {
		SetRuntimeDependencies(previousDeps)
		restoreRuntimeScopeOverrideForTest(previousScope, previousScopeSet)
	})

	globalScope := runtimeScopeFromDir(t.TempDir(), false, "/tmp/tmux/global", "100")
	SetRuntimeScope(globalScope)
	SetRuntimeDependencies(RuntimeDependencies{
		StateStoreFactory: func(scope RuntimeScope) ports.StateStorePort {
			return environmentTestStore{path: filepath.Join(scope.StateDir, "global")}
		},
	})

	envAScope := runtimeScopeFromDir(t.TempDir(), false, "/tmp/tmux/a", "111")
	envBScope := runtimeScopeFromDir(t.TempDir(), false, "/tmp/tmux/b", "222")
	envA := NewRuntimeEnvironment(RuntimeDependencies{
		StateStoreFactory: func(scope RuntimeScope) ports.StateStorePort {
			return environmentTestStore{path: filepath.Join(scope.StateDir, "env-a")}
		},
	}, envAScope)
	envB := NewRuntimeEnvironment(RuntimeDependencies{
		StateStoreFactory: func(scope RuntimeScope) ports.StateStorePort {
			return environmentTestStore{path: filepath.Join(scope.StateDir, "env-b")}
		},
	}, envBScope)

	storeA := sessionOrderStoreForEnvironment(envA)
	storeB := sessionOrderStoreForEnvironment(envB)

	if storeA.Dir() != envAScope.StateDir {
		t.Fatalf("store A dir = %q, want %q", storeA.Dir(), envAScope.StateDir)
	}
	if storeB.Dir() != envBScope.StateDir {
		t.Fatalf("store B dir = %q, want %q", storeB.Dir(), envBScope.StateDir)
	}
	if storeA.Dir() == storeB.Dir() {
		t.Fatalf("store dirs should be isolated, both were %q", storeA.Dir())
	}
	if sessionOrderStore().Dir() != globalScope.StateDir {
		t.Fatalf("default store dir = %q, want global %q", sessionOrderStore().Dir(), globalScope.StateDir)
	}
}
