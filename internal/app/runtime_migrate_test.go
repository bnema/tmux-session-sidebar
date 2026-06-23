package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/adapters/locker"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

// writeTmuxJSON writes a tmux.json to dir with the given PersistedState.
func writeTmuxJSON(t *testing.T, dir string, state ports.PersistedState) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir tmux.json dir %s: %v", dir, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmux.json"), data, 0o600); err != nil {
		t.Fatalf("write tmux.json: %v", err)
	}
}

func mustWriteRuntimeScopeMetadata(t *testing.T, scope RuntimeScope) {
	t.Helper()
	if err := os.Chmod(scope.Dir, 0o700); err != nil {
		t.Fatalf("chmod %s: %v", scope.Dir, err)
	}
	if err := writeRuntimeScopeMetadata(scope); err != nil {
		t.Fatalf("mustWriteRuntimeScopeMetadata(t, %q): %v", scope.Dir, err)
	}
}

// setMtime sets the modification time of path to unixNano.
func setMtime(t *testing.T, path string, unixNano int64) {
	t.Helper()
	tm := time.Unix(0, unixNano)
	if err := os.Chtimes(path, tm, tm); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestEnsureRuntimeStateMigrated_SameSocketNewPID(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Old scope: same socket /tmp/tmux/default, pid=111
	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		SessionOrder: []string{"alpha", "beta"},
		PinColors:    map[string]string{"alpha": "#38bdf8"},
		Sessions:     map[string]ports.SessionMetadata{},
	})

	// New scope: same socket, different pid=222
	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)

	// Durable tmux.json does not exist yet -> migration should copy from old PID-scoped state.
	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	// Switch to the new scope to verify migrated state is read from durable StateDir.
	SetRuntimeScope(newScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state from new scope: %v", err)
	}
	if len(state.SessionOrder) != 2 || state.SessionOrder[0] != "alpha" || state.SessionOrder[1] != "beta" {
		t.Fatalf("migrated SessionOrder = %#v, want [alpha beta]", state.SessionOrder)
	}
	if got := state.PinColors["alpha"]; got != "#38bdf8" {
		t.Fatalf("migrated PinColors[alpha] = %q, want #38bdf8", got)
	}
	if !fileExists(filepath.Join(newScope.StateDir, "tmux.json")) {
		t.Fatalf("migrated tmux.json missing from durable state dir %q", newScope.StateDir)
	}
}

func TestEnsureRuntimeStateMigratedAndLoadReturnsCurrentMeaningfulState(t *testing.T) {
	root := t.TempDir()
	scope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scope)
	writeTmuxJSON(t, scope.StateDir, ports.PersistedState{SessionOrder: []string{"current"}})

	state, loaded, err := ensureRuntimeStateMigratedAndLoad(context.Background(), scope)
	if err != nil {
		t.Fatalf("ensureRuntimeStateMigratedAndLoad error: %v", err)
	}
	if !loaded {
		t.Fatal("loaded = false, want current state returned for load reuse")
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "current" {
		t.Fatalf("SessionOrder = %#v, want [current]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigratedAndLoadReturnsMigratedCandidateState(t *testing.T) {
	root := t.TempDir()
	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{SessionOrder: []string{"old"}})
	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)

	state, loaded, err := ensureRuntimeStateMigratedAndLoad(context.Background(), newScope)
	if err != nil {
		t.Fatalf("ensureRuntimeStateMigratedAndLoad error: %v", err)
	}
	if !loaded {
		t.Fatal("loaded = false, want migrated state returned for load reuse")
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "old" {
		t.Fatalf("SessionOrder = %#v, want [old]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigratedAndLoadReturnsDefaultForMissingState(t *testing.T) {
	root := t.TempDir()
	scope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scope)

	state, loaded, err := ensureRuntimeStateMigratedAndLoad(context.Background(), scope)
	if err != nil {
		t.Fatalf("ensureRuntimeStateMigratedAndLoad error: %v", err)
	}
	if !loaded {
		t.Fatal("loaded = false, want initialized empty state for missing tmux.json")
	}
	if state.Sessions == nil {
		t.Fatal("state.Sessions not initialized")
	}
	if state.SessionOrder == nil {
		t.Fatal("state.SessionOrder not initialized")
	}
	if state.PinnedSessions == nil {
		t.Fatal("state.PinnedSessions not initialized")
	}
	if state.PinColors == nil {
		t.Fatal("state.PinColors not initialized")
	}
	if state.Clients == nil {
		t.Fatal("state.Clients not initialized")
	}
	if state.Heat == nil {
		t.Fatal("state.Heat not initialized")
	}
	if state.AgentAttention == nil {
		t.Fatal("state.AgentAttention not initialized")
	}
	if state.Metadata == nil {
		t.Fatal("state.Metadata not initialized")
	}
}

func TestEnsureRuntimeStateMigratedAndLoadDefersMalformedCurrentStateToCanonicalLoad(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	scope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scope)
	if err := os.MkdirAll(scope.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scope.StateDir, "tmux.json"), []byte("{invalid json"), 0o600); err != nil {
		t.Fatalf("write malformed tmux.json: %v", err)
	}

	state, loaded, err := ensureRuntimeStateMigratedAndLoad(context.Background(), scope)
	if err != nil {
		t.Fatalf("ensureRuntimeStateMigratedAndLoad error = %v, want defer to canonical load", err)
	}
	if loaded {
		t.Fatalf("loaded = true with state %#v, want false for malformed current state", state)
	}

	SetRuntimeScope(scope)
	if _, err := loadSidebarState(context.Background()); err == nil {
		t.Fatal("loadSidebarState error = nil, want canonical parse error for malformed current state")
	}
}

func TestEnsureRuntimeStateMigrated_CurrentPIDLegacyState(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	scope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scope)
	writeTmuxJSON(t, scope.Dir, ports.PersistedState{
		SessionOrder: []string{"alpha"},
		PinColors:    map[string]string{"alpha": "#38bdf8"},
	})

	if err := ensureRuntimeStateMigrated(context.Background(), scope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(scope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got := state.PinColors["alpha"]; got != "#38bdf8" {
		t.Fatalf("PinColors[alpha] = %q, want #38bdf8", got)
	}
	if !fileExists(filepath.Join(scope.StateDir, "tmux.json")) {
		t.Fatalf("expected current PID legacy state to migrate into %q", scope.StateDir)
	}
}

func TestEnsureRuntimeStateMigratedRejectsUnsafeStateDir(t *testing.T) {
	root := t.TempDir()
	scope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scope.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(scope.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ensureRuntimeStateMigrated(context.Background(), scope)
	if err == nil {
		t.Fatal("ensureRuntimeStateMigrated() error = nil, want unsafe StateDir error")
	}
}

func TestEnsureRuntimeStateMigrated_DifferentSocketIgnored(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Scope A: socket /tmp/tmux/a, pid=111 — different socket
	scopeA := runtimeScopeFromDir(root, false, "/tmp/tmux/a", "111")
	if err := os.MkdirAll(scopeA.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scopeA)
	writeTmuxJSON(t, scopeA.Dir, ports.PersistedState{
		SessionOrder: []string{"from-a"},
	})

	// Scope B: socket /tmp/tmux/default, pid=222 — target socket
	scopeB := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(scopeB.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scopeB)
	writeTmuxJSON(t, scopeB.Dir, ports.PersistedState{
		SessionOrder: []string{"from-b"},
	})

	// Scope C: same socket as B, pid=333 — new scope
	scopeC := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "333")
	if err := os.MkdirAll(scopeC.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scopeC)

	if err := ensureRuntimeStateMigrated(context.Background(), scopeC); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	// Must have migrated from B (same socket), not from A (different socket).
	SetRuntimeScope(scopeC)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "from-b" {
		t.Fatalf("migrated SessionOrder = %#v, want [from-b]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigrated_MalformedCurrentNotOverwrittenByCandidate(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{SessionOrder: []string{"old-data"}})

	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)
	if err := os.MkdirAll(newScope.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	currentPath := filepath.Join(newScope.StateDir, "tmux.json")
	if err := os.WriteFile(currentPath, []byte("{invalid json"), 0o600); err != nil {
		t.Fatalf("write malformed current tmux.json: %v", err)
	}

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}
	data, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current tmux.json: %v", err)
	}
	if string(data) != "{invalid json" {
		t.Fatalf("current tmux.json was overwritten with %q", data)
	}
	SetRuntimeScope(newScope)
	if _, err := loadSidebarState(context.Background()); err == nil {
		t.Fatal("loadSidebarState error = nil, want canonical parse error for malformed current state")
	}
}

func TestEnsureRuntimeStateMigrated_CurrentMeaningfulNotOverwritten(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		SessionOrder: []string{"old-data"},
	})

	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)
	// Already has meaningful durable state — should not be overwritten.
	writeTmuxJSON(t, newScope.StateDir, ports.PersistedState{
		SessionOrder: []string{"current-state"},
		Sessions:     map[string]ports.SessionMetadata{},
	})

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(newScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "current-state" {
		t.Fatalf("overwritten SessionOrder = %#v, want [current-state]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigrated_EmptyPriorIgnored(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Prior scope with empty/default tmux.json.
	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		SessionOrder: []string{},
		Sessions:     map[string]ports.SessionMetadata{},
	})

	// New scope with no tmux.json.
	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	// No migration should have occurred since the candidate was not meaningful.
	// New scope should still have no tmux.json or empty one.
	if fileExists(filepath.Join(newScope.StateDir, "tmux.json")) {
		t.Fatal("expected no durable tmux.json to be created for empty prior")
	}
}

func TestEnsureRuntimeStateMigrated_EmptyDefaultJSONIgnored(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Prior scope with an explicitly empty JSON object (valid but not meaningful).
	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	// Write an empty JSON object — valid JSON, zero values.
	if err := os.WriteFile(filepath.Join(oldScope.Dir, "tmux.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	if fileExists(filepath.Join(newScope.StateDir, "tmux.json")) {
		t.Fatal("expected no durable tmux.json for empty JSON prior")
	}
}

func TestEnsureRuntimeStateMigrated_StartupOnlyStateDoesNotBlockMigration(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{SessionOrder: []string{"alpha"}})

	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)
	writeTmuxJSON(t, newScope.Dir, ports.PersistedState{SessionOrder: []string{"__startup", "123"}})

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(newScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "alpha" {
		t.Fatalf("migrated SessionOrder = %#v, want [alpha]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigrated_MultipleCandidatesChooseNewest(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Older candidate.
	olderScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(olderScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, olderScope)
	writeTmuxJSON(t, olderScope.Dir, ports.PersistedState{
		SessionOrder: []string{"older"},
	})
	setMtime(t, filepath.Join(olderScope.Dir, "tmux.json"), 1_000_000_000_000) // 2001-09-09

	// Newer candidate.
	newerScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newerScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newerScope)
	writeTmuxJSON(t, newerScope.Dir, ports.PersistedState{
		SessionOrder: []string{"newer"},
	})
	setMtime(t, filepath.Join(newerScope.Dir, "tmux.json"), 2_000_000_000_000) // 2033-05-18

	// Target scope.
	targetScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "333")
	if err := os.MkdirAll(targetScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, targetScope)

	if err := ensureRuntimeStateMigrated(context.Background(), targetScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(targetScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "newer" {
		t.Fatalf("migrated SessionOrder = %#v, want [newer]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigrated_IgnoresNewerStartupOnlyCandidate(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	olderRealScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(olderRealScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, olderRealScope)
	writeTmuxJSON(t, olderRealScope.Dir, ports.PersistedState{SessionOrder: []string{"alpha"}})
	setMtime(t, filepath.Join(olderRealScope.Dir, "tmux.json"), 1_000_000_000_000)

	newerStartupScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newerStartupScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newerStartupScope)
	writeTmuxJSON(t, newerStartupScope.Dir, ports.PersistedState{SessionOrder: []string{"__startup", "123"}})
	setMtime(t, filepath.Join(newerStartupScope.Dir, "tmux.json"), 2_000_000_000_000)

	targetScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "333")
	if err := os.MkdirAll(targetScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, targetScope)

	if err := ensureRuntimeStateMigrated(context.Background(), targetScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(targetScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "alpha" {
		t.Fatalf("migrated SessionOrder = %#v, want [alpha]", state.SessionOrder)
	}
}

func TestEnsureRuntimeStateMigrated_LegacyNotMigrated(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Legacy scope has Legacy=true and no server identity.
	legacyScope := runtimeScopeFromDir(root, true, "", "")
	if err := os.MkdirAll(legacyScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, legacyScope)

	// Scoped sibling with same effective socket should be skipped because
	// the legacy scope's SocketPath is empty and IdentityKey is empty.
	scopedScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(scopedScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, scopedScope)
	writeTmuxJSON(t, scopedScope.Dir, ports.PersistedState{
		SessionOrder: []string{"scoped-data"},
	})

	if err := ensureRuntimeStateMigrated(context.Background(), legacyScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	// Legacy scope should remain untouched (no tmux.json copied).
	if fileExists(filepath.Join(legacyScope.Dir, "tmux.json")) {
		t.Fatal("legacy scope should not receive migrated tmux.json")
	}
}

// Test the guard: missing server.json means sibling is silently skipped.
func TestEnsureRuntimeStateMigrated_MissingServerJSON(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Sibling dir with tmux.json but no server.json.
	siblingDir := filepath.Join(root, "servers", "somehash")
	if err := os.MkdirAll(siblingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTmuxJSON(t, siblingDir, ports.PersistedState{
		SessionOrder: []string{"orphan-data"},
	})

	newScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(newScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, newScope)

	if err := ensureRuntimeStateMigrated(context.Background(), newScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	if fileExists(filepath.Join(newScope.StateDir, "tmux.json")) {
		t.Fatal("orphan sibling without server.json should be silently skipped")
	}
}

// Test that sidebar layout with session refs is considered meaningful.
func TestEnsureRuntimeStateMigrated_MeaningfulSidebarLayout(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	// State with no Sessions or SessionOrder but a layout with session refs.
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{},
		Sidebar:  &ports.SidebarState{Open: true},
		SidebarLayout: &ports.SidebarLayout{
			Items: []ports.SidebarLayoutItem{
				{Kind: "category", Category: &ports.SidebarLayoutCategory{
					Name:     "work",
					Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}},
				}},
			},
		},
	})

	// Sibling with same socket but different PID and empty state (should be skipped).
	emptyScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(emptyScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, emptyScope)
	writeTmuxJSON(t, emptyScope.Dir, ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{},
	})

	targetScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "333")
	if err := os.MkdirAll(targetScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, targetScope)

	if err := ensureRuntimeStateMigrated(context.Background(), targetScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(targetScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Sidebar == nil {
		t.Fatal("migrated state should include sidebar state")
	}
	if !state.Sidebar.Open {
		t.Fatal("migrated state should have Open=true")
	}
	if state.SidebarLayout == nil || len(state.SidebarLayout.Items) != 1 {
		t.Fatalf("migrated SidebarLayout has %d items, want 1", len(state.SidebarLayout.Items))
	}
	if state.SidebarLayout.Items[0].Category == nil || len(state.SidebarLayout.Items[0].Category.Sessions) != 1 {
		t.Fatal("migrated layout category should have 1 session ref")
	}
	if state.SidebarLayout.Items[0].Category.Sessions[0].Name != "alpha" {
		t.Fatalf("migrated session ref name = %q, want alpha", state.SidebarLayout.Items[0].Category.Sessions[0].Name)
	}
}

// TestEnsureRuntimeStateMigrated_RecheckCatchesLateState verifies that the
// re-check under the state lock correctly catches meaningful state written
// by a concurrent process between the initial meaningfulness check and the
// final migration write. The test holds the state lock externally while the
// migration goroutine blocks on it after passing the initial check, then
// writes meaningful state before releasing the lock — the migration must
// re-check and skip the overwrite.
func TestEnsureRuntimeStateMigrated_RecheckCatchesLateState(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	// Old scope: meaningful candidate state.
	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		SessionOrder: []string{"old-data"},
	})

	// Another old scope (also meaningful) to add slight delay to
	// candidate scanning, increasing the chance the goroutine reaches
	// lock acquisition before the test writes concurrent state.
	extraOld := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "100")
	if err := os.MkdirAll(extraOld.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, extraOld)
	writeTmuxJSON(t, extraOld.Dir, ports.PersistedState{
		SessionOrder: []string{"extra-old-data"},
	})

	// Target scope: no tmux.json yet (initial check will pass).
	targetScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(targetScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, targetScope)

	// Acquire the state lock externally. The migration goroutine will
	// block trying to acquire this same lock after passing the initial
	// check and finding candidates.
	stateLock, err := (locker.FileLocker{Dir: filepath.Join(targetScope.StateDir, "locks")}).Acquire(
		context.Background(), "tmux-sidebar-state",
	)
	if err != nil {
		t.Fatalf("acquire external state lock: %v", err)
	}

	reachedPreLock := make(chan struct{})
	beforeMigrationStateLockForTest = func() { close(reachedPreLock) }
	defer func() { beforeMigrationStateLockForTest = nil }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ensureRuntimeStateMigrated(context.Background(), targetScope)
	}()

	// Wait until migration has passed the initial meaningfulness check,
	// selected/read its candidate, and reached the boundary immediately
	// before acquiring the state lock. This deterministically exercises
	// the under-lock recheck instead of relying on scheduler timing.
	select {
	case <-reachedPreLock:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for migration pre-lock hook")
	}

	// While the goroutine is blocked on the state lock, write meaningful
	// state to the target scope — this simulates the race where another
	// startup command creates tmux.json between our candidate selection
	// and the protected write.
	writeTmuxJSON(t, targetScope.StateDir, ports.PersistedState{
		SessionOrder: []string{"concurrent-write"},
	})

	// Release the external lock. The goroutine's Acquire returns, and
	// the re-check should see the newly-written meaningful state and
	// skip the overwrite.
	if err := stateLock.Release(); err != nil {
		t.Fatalf("release external state lock: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ensureRuntimeStateMigrated")
	}

	// Verify the concurrent write was preserved, not overwritten with
	// the stale candidate data.
	SetRuntimeScope(targetScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "concurrent-write" {
		t.Fatalf("SessionOrder = %#v, want [concurrent-write]", state.SessionOrder)
	}
}

// TestEnsureRuntimeStateMigrated_NewStateBeforeLockSkips verifies that
// when meaningful state already exists before ensureRuntimeStateMigrated
// is called (the common happy path), migration correctly skips and does
// not acquire the state lock at all (the initial check bails out early).
func TestEnsureRuntimeStateMigrated_NewStateBeforeLockSkips(t *testing.T) {
	root := t.TempDir()
	defer ResetRuntimeScopeForTest()

	oldScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "111")
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, oldScope)
	writeTmuxJSON(t, oldScope.Dir, ports.PersistedState{
		SessionOrder: []string{"old-data"},
	})

	targetScope := runtimeScopeFromDir(root, false, "/tmp/tmux/default", "222")
	if err := os.MkdirAll(targetScope.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteRuntimeScopeMetadata(t, targetScope)
	// Target already has meaningful durable state before migration runs.
	writeTmuxJSON(t, targetScope.StateDir, ports.PersistedState{
		SessionOrder: []string{"current-state"},
	})

	if err := ensureRuntimeStateMigrated(context.Background(), targetScope); err != nil {
		t.Fatalf("ensureRuntimeStateMigrated() error = %v", err)
	}

	SetRuntimeScope(targetScope)
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "current-state" {
		t.Fatalf("SessionOrder = %#v, want [current-state]", state.SessionOrder)
	}
}
