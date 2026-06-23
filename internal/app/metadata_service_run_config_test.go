package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type metadataFakeConfig struct {
	mu       sync.Mutex
	snapshot ports.ConfigSnapshot
	errs     []error
}

func (c *metadataFakeConfig) LoadConfig(ctx context.Context) (ports.ConfigSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		if err != nil {
			return ports.ConfigSnapshot{}, err
		}
	}
	return c.snapshot, nil
}

func (c *metadataFakeConfig) set(snapshot ports.ConfigSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = snapshot
}

type metadataLifecycleWatcher struct {
	mu     sync.Mutex
	starts int
	stops  int
}

func newMetadataLifecycleWatcher() *metadataLifecycleWatcher { return &metadataLifecycleWatcher{} }

func (w *metadataLifecycleWatcher) Watch(ctx context.Context, paths []string) (<-chan ports.FileWatchEvent, <-chan error, error) {
	w.mu.Lock()
	w.starts++
	w.mu.Unlock()
	events := make(chan ports.FileWatchEvent)
	errs := make(chan error)
	go func() {
		<-ctx.Done()
		w.mu.Lock()
		w.stops++
		w.mu.Unlock()
		close(events)
		close(errs)
	}()
	return events, errs, nil
}

func (w *metadataLifecycleWatcher) waitStarts(t *testing.T, want int) {
	t.Helper()
	eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.starts >= want
	})
}

func (w *metadataLifecycleWatcher) waitStops(t *testing.T, want int) {
	t.Helper()
	eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.stops >= want
	})
}

func TestMetadataServiceRunReturnsConfigReloadError(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}}},
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main"}},
	}
	wantErr := errors.New("temporary config read failed")
	config := &metadataFakeConfig{snapshot: ports.ConfigSnapshot{MetadataSublineEnabled: true}, errs: []error{nil, wantErr}}
	watcher := newMetadataLifecycleWatcher()
	reconcile := make(chan struct{}, 1)
	svc := MetadataService{Store: store, Query: tmux, Git: git, Watcher: watcher, Config: config, ReconcileRequests: reconcile, LockStore: metadataDirectLock(store), ReconcileInterval: time.Hour}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true}) }()
	watcher.waitStarts(t, 1)

	reconcile <- struct{}{}
	watcher.waitStops(t, 1)
	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("Run error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return config reload error")
	}
}

func TestMetadataServiceRunStopsWatcherWhenConfigDisablesMetadata(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}}},
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main"}},
	}
	config := &metadataFakeConfig{snapshot: ports.ConfigSnapshot{MetadataSublineEnabled: true}}
	watcher := newMetadataLifecycleWatcher()
	reconcile := make(chan struct{}, 1)
	svc := MetadataService{Store: store, Query: tmux, Git: git, Watcher: watcher, Config: config, ReconcileRequests: reconcile, LockStore: metadataDirectLock(store), ReconcileInterval: time.Hour}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true}) }()
	watcher.waitStarts(t, 1)

	config.set(ports.ConfigSnapshot{MetadataSublineEnabled: false})
	reconcile <- struct{}{}
	watcher.waitStops(t, 1)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

func TestMetadataServiceRunRestartsWatcherWhenConfigReenablesMetadata(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}}},
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main"}},
	}
	config := &metadataFakeConfig{snapshot: ports.ConfigSnapshot{MetadataSublineEnabled: true}}
	watcher := newMetadataLifecycleWatcher()
	reconcile := make(chan struct{}, 2)
	svc := MetadataService{Store: store, Query: tmux, Git: git, Watcher: watcher, Config: config, ReconcileRequests: reconcile, LockStore: metadataDirectLock(store), ReconcileInterval: time.Hour}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true}) }()
	watcher.waitStarts(t, 1)

	config.set(ports.ConfigSnapshot{MetadataSublineEnabled: false})
	reconcile <- struct{}{}
	watcher.waitStops(t, 1)
	config.set(ports.ConfigSnapshot{MetadataSublineEnabled: true})
	reconcile <- struct{}{}
	watcher.waitStarts(t, 2)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}
