package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/gitcli"
	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/ports"
)

const metadataGitStatusConcurrency = 4 // Keep async Git I/O bounded so sidebar refreshes do not fan out unbounded subprocesses.

var metadataGitStatusTimeout = 250 * time.Millisecond
var metadataCaptureInFlight atomic.Bool

func captureSessionMetadataAsync(ctx context.Context, cfg ports.ConfigSnapshot) {
	if ctx.Value(disableAsyncMetadataCaptureKey{}) != nil || !cfg.MetadataSublineEnabled || !metadataCaptureInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer metadataCaptureInFlight.Store(false)
		if changed, err := captureSessionMetadata(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata capture failed: %v\n", err)
		} else if changed {
			refreshAllSidebarPanesBestEffort(ctx)
		}
	}()
}

type disableAsyncMetadataCaptureKey struct{}

func captureSessionMetadata(ctx context.Context, cfg ports.ConfigSnapshot) (bool, error) {
	if !cfg.MetadataSublineEnabled {
		return false, nil
	}
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		return false, err
	}
	live, err := newTmuxClient().ListSessions(ctx)
	if err != nil {
		return false, err
	}

	livePaths := make(map[string]string, len(live))
	for _, session := range live {
		if path, err := newTmuxClient().SessionPath(ctx, session.Name); err == nil {
			livePaths[session.Name] = path
		}
	}
	liveNames := make(map[string]struct{}, len(live))
	sessionPaths := make(map[string]string, len(live))
	terminalDeletes := make(map[string]struct{})
	for _, session := range live {
		liveNames[session.Name] = struct{}{}
		path, ok := sessionMetadataCapturePath(session.Name, state.Sessions[session.Name], livePaths)
		if !ok {
			terminalDeletes[session.Name] = struct{}{}
			continue
		}
		sessionPaths[session.Name] = path
	}

	results := make(map[string]ports.GitStatus, len(sessionPaths))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, metadataGitStatusConcurrency)
	git := gitcli.Git{Process: process.Runner{}}
	for sessionName, path := range sessionPaths {
		wg.Add(1)
		go func(sessionName string, path string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			statusCtx, cancel := context.WithTimeout(ctx, metadataGitStatusTimeout)
			defer cancel()
			status, err := git.Status(statusCtx, path)
			if err != nil {
				if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
					mu.Lock()
					terminalDeletes[sessionName] = struct{}{}
					mu.Unlock()
					return
				}
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: git status failed for session %q path %q: %v\n", sessionName, path, err)
				return
			}
			mu.Lock()
			results[sessionName] = status
			mu.Unlock()
		}(sessionName, path)
	}
	wg.Wait()

	changed := false
	err = withLockedSidebarStore(ctx, func(store storefs.Store) error {
		latest, err := store.Load(ctx, "tmux")
		if err != nil {
			return err
		}
		next := maps.Clone(latest.Metadata)
		if next == nil {
			next = map[string]ports.GitStatus{}
		}
		for name := range liveNames {
			if _, ok := latest.Sessions[name]; !ok {
				delete(next, name)
				continue
			}
			if _, ok := sessionMetadataPath(latest.Sessions[name]); !ok {
				delete(next, name)
				continue
			}
			if status, ok := results[name]; ok {
				next[name] = status
				continue
			}
			if _, ok := terminalDeletes[name]; ok {
				delete(next, name)
			}
		}
		if gitMetadataEqual(latest.Metadata, next) {
			return nil
		}
		latest.Metadata = next
		changed = true
		return store.Save(ctx, "tmux", latest)
	})
	return changed, err
}

func sessionMetadataCapturePath(sessionName string, metadata ports.SessionMetadata, livePaths map[string]string) (string, bool) {
	if path := livePaths[sessionName]; path != "" {
		return path, true
	}
	return sessionMetadataPath(metadata)
}

func sessionMetadataPath(metadata ports.SessionMetadata) (string, bool) {
	if metadata.ProjectPath != "" {
		return metadata.ProjectPath, true
	}
	if metadata.LastPath != "" {
		return metadata.LastPath, true
	}
	return "", false
}

func gitMetadataEqual(left, right map[string]ports.GitStatus) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok || leftValue != rightValue {
			return false
		}
	}
	return true
}
