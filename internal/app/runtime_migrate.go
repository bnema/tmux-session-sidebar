package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bnema/tmux-session-sidebar/internal/core/persisted"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

// stateCandidate describes a sibling scope whose tmux.json is a candidate for
// migration to the current scope.
type stateCandidate struct {
	dir          string
	tmuxJSONPath string
	mtime        int64 // unix nano for deterministic comparison
}

// ensureRuntimeStateMigrated copies tmux.json from the old PID-scoped server
// directory into the durable socket-scoped state directory. It only migrates
// when the durable tmux.json is missing or not meaningful, preserving any
// active state.
//
// It internally acquires the tmux-sidebar-state lock for the durable state
// directory to prevent a check-then-write race with concurrent startup commands
// that may write meaningful state into tmux.json between candidate selection
// and the final copy.
func ensureRuntimeStateMigrated(ctx context.Context, scope RuntimeScope) error {
	if err := EnsureRuntimeDirPrivate(scope.StateDir); err != nil {
		return err
	}
	// Only scoped (non-legacy) runtimes participate in migration. Legacy
	// state is always at the root and carries no server identity.
	if scope.Legacy || scope.IdentityKey == "" {
		return nil
	}

	currentPath := filepath.Join(scope.StateDir, "tmux.json")

	// Quick bail-out: if current tmux.json already has meaningful state,
	// do nothing. The true serialised check happens below under the state
	// lock, but this avoids an unnecessary sibling scan in the common case.
	meaningful, err := tmuxJSONIsMeaningful(currentPath)
	if err != nil {
		return fmt.Errorf("check current tmux.json: %w", err)
	}
	if meaningful {
		return nil
	}

	// Find old PID-scoped server directories sharing the same socket path.
	candidates, err := findSiblingStateCandidates(scope)
	if err != nil {
		return fmt.Errorf("find sibling state candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}

	// Pick the newest candidate by file modification time, with a stable
	// path tie-breaker for filesystems with coarse timestamp precision.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].mtime == candidates[j].mtime {
			return candidates[i].tmuxJSONPath < candidates[j].tmuxJSONPath
		}
		return candidates[i].mtime > candidates[j].mtime
	})

	if beforeMigrationStateLockForTest != nil {
		beforeMigrationStateLockForTest()
	}

	// Acquire the same state-level lock used by withLockedSidebarStore so
	// that no concurrent startup command can write meaningful tmux.json into
	// our current scope between our meaningfulness check and the final copy.
	lock, err := runtimeLocker(filepath.Join(scope.StateDir, "locks")).Acquire(ctx, "tmux-sidebar-state")
	if err != nil {
		return fmt.Errorf("acquire state lock for migration: %w", err)
	}
	defer func() {
		if relErr := lock.Release(); relErr != nil {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: release migration lock: %v\n", relErr)
		}
	}()

	// Re-check: another process may have written meaningful state between
	// the initial check above and acquiring the lock. If so, do not
	// overwrite it with stale candidate data.
	meaningful, err = tmuxJSONIsMeaningful(currentPath)
	if err != nil {
		return fmt.Errorf("recheck current tmux.json under lock: %w", err)
	}
	if meaningful {
		return nil
	}

	best := candidates[0]
	data, err := os.ReadFile(best.tmuxJSONPath)
	if err != nil {
		return fmt.Errorf("read candidate tmux.json %s under lock: %w", best.tmuxJSONPath, err)
	}
	meaningful, err = stateDataIsMeaningful(data)
	if err != nil {
		return fmt.Errorf("check candidate tmux.json %s under lock: %w", best.tmuxJSONPath, err)
	}
	if !meaningful {
		return nil
	}

	// Atomic write via temp file + rename to prevent partial-file reads.
	if err := atomicWriteFile(currentPath, data, 0o600); err != nil {
		return fmt.Errorf("write migrated tmux.json: %w", err)
	}
	return nil
}

// findSiblingStateCandidates returns old PID-scoped server directories whose
// tmux.json may be migrated into the durable socket-scoped state directory.
func findSiblingStateCandidates(scope RuntimeScope) ([]stateCandidate, error) {
	serversDir := filepath.Join(scope.RootDir, "servers")
	entries, err := os.ReadDir(serversDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return oldStateCandidateForDir(scope.Dir), nil
		}
		return nil, err
	}

	canonicalCurrentSocket := canonicalPath(scope.SocketPath)
	candidates := oldStateCandidateForDir(scope.Dir)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		siblingDir := filepath.Join(serversDir, entry.Name())
		if siblingDir == scope.Dir {
			continue
		}

		meta, err := readServerMetadata(filepath.Join(siblingDir, "server.json"))
		if err != nil {
			continue // skip unreadable or missing server.json
		}
		if meta.Legacy || meta.SocketPath == "" {
			continue
		}
		if canonicalPath(meta.SocketPath) != canonicalCurrentSocket {
			continue
		}

		candidates = append(candidates, oldStateCandidateForDir(siblingDir)...)
	}
	return candidates, nil
}

func oldStateCandidateForDir(dir string) []stateCandidate {
	tmuxPath := filepath.Join(dir, "tmux.json")
	info, err := os.Stat(tmuxPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	meaningful, err := tmuxJSONIsMeaningful(tmuxPath)
	if err != nil || !meaningful {
		return nil
	}
	return []stateCandidate{{dir: dir, tmuxJSONPath: tmuxPath, mtime: info.ModTime().UnixNano()}}
}

// tmuxJSONIsMeaningful returns true if the file at path exists, is valid JSON,
// and contains meaningful PersistedState beyond the empty default.
func tmuxJSONIsMeaningful(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return stateDataIsMeaningful(data)
}

func stateDataIsMeaningful(data []byte) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}

	var state ports.PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, nil // unparseable is not meaningful
	}

	return persisted.IsMeaningful(state), nil
}

var beforeMigrationStateLockForTest func()

// readServerMetadata reads and unmarshals a server.json metadata file.
func readServerMetadata(path string) (runtimeScopeMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return runtimeScopeMetadata{}, err
	}
	var meta runtimeScopeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return runtimeScopeMetadata{}, err
	}
	return meta, nil
}

// atomicWriteFile writes data to path atomically by writing to a temporary
// file in the same directory and then renaming it over the target. This
// prevents partial-file reads by concurrent readers.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanedUp := false
	defer func() {
		if !cleanedUp {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanedUp = true
	return nil
}
