package app

import (
	"context"
	"errors"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestMetadataServiceCaptureAcceptsFreshLiveResultWithoutPersistedSession(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{},
		Metadata: map[string]ports.GitStatus{"gone": {RepoRoot: "/old", Branch: "main", Modified: 1}},
	}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "feat", Modified: 1}}}
	svc := MetadataService{
		Store:                store,
		Tmux:                 tmux,
		Git:                  git,
		LockStore:            metadataDirectLock(store),
		GitStatusTimeout:     time.Second,
		GitStatusConcurrency: 1,
	}

	changed, err := svc.Capture(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if err != nil {
		t.Fatalf("Capture error: %v", err)
	}
	if !changed {
		t.Fatal("Capture changed = false, want true")
	}
	if got := store.state.Metadata["alpha"]; got.RepoRoot != "/repo" || got.Modified != 1 {
		t.Fatalf("Metadata[alpha] = %#v, want fresh git status", got)
	}
	if _, ok := store.state.Metadata["gone"]; ok {
		t.Fatalf("stale metadata for dead session kept: %#v", store.state.Metadata)
	}
}

func TestMetadataServiceCaptureAndRefreshRefreshesOnlyWhenMetadataChanges(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "feat", Modified: 1}}}
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, Refresher: refresher, LockStore: metadataDirectLock(store), GitStatusTimeout: time.Second, GitStatusConcurrency: 1}

	if err := svc.CaptureAndRefresh(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true}); err != nil {
		t.Fatalf("CaptureAndRefresh first error: %v", err)
	}
	if refresher.callCount() != 1 {
		t.Fatalf("refresh calls after changed capture = %d, want 1", refresher.callCount())
	}
	if err := svc.CaptureAndRefresh(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true}); err != nil {
		t.Fatalf("CaptureAndRefresh second error: %v", err)
	}
	if refresher.callCount() != 1 {
		t.Fatalf("refresh calls after unchanged capture = %d, want still 1", refresher.callCount())
	}
}

type metadataFakeStore struct {
	mu    sync.Mutex
	state ports.PersistedState
}

func (s *metadataFakeStore) Load(ctx context.Context, serverID string) (ports.PersistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMetadataState(s.state), nil
}

func (s *metadataFakeStore) Save(ctx context.Context, serverID string, state ports.PersistedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = cloneMetadataState(state)
	return nil
}

func (s *metadataFakeStore) metadata(name string) ports.GitStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Metadata[name]
}

func cloneMetadataState(state ports.PersistedState) ports.PersistedState {
	cloned := state
	if state.Sessions != nil {
		cloned.Sessions = make(map[string]ports.SessionMetadata, len(state.Sessions))
		maps.Copy(cloned.Sessions, state.Sessions)
	}
	if state.Metadata != nil {
		cloned.Metadata = make(map[string]ports.GitStatus, len(state.Metadata))
		maps.Copy(cloned.Metadata, state.Metadata)
	}
	return cloned
}

func metadataDirectLock(store ports.StateStorePort) func(context.Context, func(ports.StateStorePort) error) error {
	return func(ctx context.Context, fn func(ports.StateStorePort) error) error {
		return fn(store)
	}
}

type metadataFakeTmux struct {
	sessions []ports.TmuxSessionSnapshot
	paths    map[string]string
}

func (t metadataFakeTmux) ServerID(ctx context.Context) (string, error) { return "tmux", nil }
func (t metadataFakeTmux) ListSessions(ctx context.Context) ([]ports.TmuxSessionSnapshot, error) {
	return t.sessions, nil
}
func (t metadataFakeTmux) ListClients(ctx context.Context) ([]ports.TmuxClientSnapshot, error) {
	return nil, nil
}
func (t metadataFakeTmux) CurrentPanePath(ctx context.Context, clientID string) (string, error) {
	return "", nil
}
func (t metadataFakeTmux) SessionPath(ctx context.Context, sessionName string) (string, error) {
	return t.paths[sessionName], nil
}
func (t metadataFakeTmux) PaneSize(ctx context.Context, paneID string) (ports.PaneSize, error) {
	return ports.PaneSize{}, nil
}

type metadataFakeGit struct {
	statuses    map[string]ports.GitStatus
	infos       map[string]ports.GitRepoInfo
	targets     map[string]ports.GitWatchTargets
	statusCalls *atomic.Int64
	statusErr   error
}

func (g metadataFakeGit) RepoRoot(ctx context.Context, path string) (string, error) {
	if info, ok := g.infos[path]; ok {
		return info.RepoRoot, nil
	}
	return g.statuses[path].RepoRoot, nil
}
func (g metadataFakeGit) Status(ctx context.Context, path string) (ports.GitStatus, error) {
	if g.statusCalls != nil {
		g.statusCalls.Add(1)
	}
	if g.statusErr != nil {
		return ports.GitStatus{}, g.statusErr
	}
	return g.statuses[path], nil
}
func (g metadataFakeGit) RepoInfo(ctx context.Context, path string) (ports.GitRepoInfo, error) {
	if info, ok := g.infos[path]; ok {
		return info, nil
	}
	status := g.statuses[path]
	return ports.GitRepoInfo{RepoRoot: status.RepoRoot, WorktreeRoot: status.RepoRoot, Branch: status.Branch}, nil
}
func (g metadataFakeGit) WatchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	if targets, ok := g.targets[path]; ok {
		return targets, nil
	}
	status := g.statuses[path]
	return ports.GitWatchTargets{RepoRoot: status.RepoRoot, WorktreeRoot: status.RepoRoot}, nil
}

type metadataFakeRefresher struct{ calls atomic.Int64 }

func (r *metadataFakeRefresher) RefreshAllSidebars(ctx context.Context) error {
	r.calls.Add(1)
	return nil
}

func (r *metadataFakeRefresher) callCount() int64 {
	return r.calls.Load()
}

func TestMetadataServiceReconcileBuildsRepoSubscriptions(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}, paths: map[string]string{"alpha": "/repo/a", "beta": "/repo/b"}}
	git := metadataFakeGit{infos: map[string]ports.GitRepoInfo{
		"/repo/a": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"},
		"/repo/b": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"},
	}, targets: map[string]ports.GitWatchTargets{
		"/repo/a": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}},
		"/repo/b": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}},
	}}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, LockStore: metadataDirectLock(store)}

	subs, err := svc.Reconcile(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("subscriptions len = %d, want 1: %#v", len(subs), subs)
	}
	if got := subs["/repo"].SessionNames; len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("subscription sessions = %#v, want alpha beta", got)
	}
}

func TestMetadataServiceCaptureRepoUpdatesMappedSessionsAndRefreshesOnChange(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{"alpha": {RepoRoot: "/repo", Branch: "main", Clean: true}}}}
	git := metadataFakeGit{statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}}}
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{Store: store, Git: git, Refresher: refresher, LockStore: metadataDirectLock(store)}
	sub := MetadataRepoSubscription{RepoRoot: "/repo", WorktreeRoot: "/repo", SessionNames: []string{"alpha", "beta"}}

	changed, err := svc.CaptureRepo(t.Context(), sub)
	if err != nil {
		t.Fatalf("CaptureRepo error: %v", err)
	}
	if !changed {
		t.Fatal("CaptureRepo changed = false, want true")
	}
	if store.metadata("alpha").Modified != 1 || store.metadata("beta").Modified != 1 {
		t.Fatalf("Metadata after CaptureRepo = %#v", store.state.Metadata)
	}
	if refresher.callCount() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refresher.callCount())
	}
}

func TestMetadataServiceRunWatchesDebouncesAndCapturesRepo(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}}},
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}},
	}
	watcher := newMetadataFakeWatcher()
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, Watcher: watcher, Refresher: refresher, LockStore: metadataDirectLock(store), Debounce: time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true}) }()
	watcher.waitStarted(t)

	watcher.events <- ports.FileWatchEvent{Path: "/repo/file.go", Op: "WRITE"}
	eventually(t, func() bool { return refresher.callCount() == 1 && store.metadata("alpha").Modified == 1 })
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

type metadataFakeWatcher struct {
	events  chan ports.FileWatchEvent
	errs    chan error
	started chan struct{}
}

func newMetadataFakeWatcher() *metadataFakeWatcher {
	return &metadataFakeWatcher{events: make(chan ports.FileWatchEvent, 8), errs: make(chan error, 1), started: make(chan struct{})}
}

func (w *metadataFakeWatcher) Watch(ctx context.Context, paths []string) (<-chan ports.FileWatchEvent, <-chan error, error) {
	close(w.started)
	return w.events, w.errs, nil
}

func (w *metadataFakeWatcher) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-w.started:
	case <-time.After(time.Second):
		t.Fatal("watcher did not start")
	}
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition was not met before timeout")
		case <-tick.C:
		}
	}
}

func TestMetadataServiceRunCoalescesBurstEventsIntoOneRepoCapture(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	var statusCalls atomic.Int64
	git := metadataFakeGit{
		infos:       map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:     map[string]ports.GitWatchTargets{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo", Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo"}}},
		statuses:    map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}},
		statusCalls: &statusCalls,
	}
	watcher := newMetadataFakeWatcher()
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, Watcher: watcher, Refresher: refresher, LockStore: metadataDirectLock(store), Debounce: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true}) }()
	watcher.waitStarted(t)

	watcher.events <- ports.FileWatchEvent{Path: "/repo/file.go", Op: "WRITE"}
	watcher.events <- ports.FileWatchEvent{Path: "/repo/other.go", Op: "WRITE"}
	eventually(t, func() bool { return refresher.callCount() == 1 })
	if statusCalls.Load() != 1 {
		t.Fatalf("git status calls = %d, want one coalesced capture", statusCalls.Load())
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

func TestMetadataServiceCaptureRepoDeletesMetadataWhenRepoDisappears(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{"alpha": {RepoRoot: "/repo", Branch: "main", Modified: 1}}}}
	git := metadataFakeGit{statusErr: ports.ErrNotGitRepository}
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{Store: store, Git: git, Refresher: refresher, LockStore: metadataDirectLock(store)}
	sub := MetadataRepoSubscription{RepoRoot: "/repo", WorktreeRoot: "/repo", SessionNames: []string{"alpha"}}

	changed, err := svc.CaptureRepo(t.Context(), sub)
	if err != nil {
		t.Fatalf("CaptureRepo error: %v", err)
	}
	if !changed {
		t.Fatal("CaptureRepo changed = false, want true")
	}
	if got := store.metadata("alpha"); got != (ports.GitStatus{}) {
		t.Fatalf("metadata alpha kept after terminal delete: %#v", got)
	}
	if refresher.callCount() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refresher.callCount())
	}
}
