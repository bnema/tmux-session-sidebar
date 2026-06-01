package app

import (
	"context"
	"errors"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/gitcli"
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

func TestMetadataServiceDefaultGitStatusTimeoutAllowsModeratelySlowRepositories(t *testing.T) {
	svc := MetadataService{}
	if got := svc.gitStatusTimeout(); got < time.Second {
		t.Fatalf("default git status timeout = %s, want at least 1s", got)
	}
}

func TestNewMetadataServiceUsesCLIStatusBackend(t *testing.T) {
	svc := NewMetadataService()
	git, ok := svc.Git.(metadataGit)
	if !ok {
		t.Fatalf("NewMetadataService Git backend = %T, want metadataGit", svc.Git)
	}
	if _, ok := git.StatusGit.(gitcli.Git); !ok {
		t.Fatalf("metadata status backend = %T, want gitcli.Git", git.StatusGit)
	}
	if _, ok := git.RepoGit.(gitcli.Git); ok {
		t.Fatalf("metadata repo/watch backend = gitcli.Git, want non-CLI backend to avoid reconcile subprocess fanout")
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
	infoErrs    map[string]error
	targetErrs  map[string]error
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
	if err, ok := g.infoErrs[path]; ok {
		return ports.GitRepoInfo{}, err
	}
	if info, ok := g.infos[path]; ok {
		return info, nil
	}
	status := g.statuses[path]
	return ports.GitRepoInfo{RepoRoot: status.RepoRoot, WorktreeRoot: status.RepoRoot, Branch: status.Branch}, nil
}
func (g metadataFakeGit) WatchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	if err, ok := g.targetErrs[path]; ok {
		return ports.GitWatchTargets{}, err
	}
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

func TestMetadataServiceCaptureRepoCoolsDownAfterDeadlineExceeded(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{"alpha": {RepoRoot: "/repo", Branch: "main", Clean: true}}}}
	now := time.Unix(100, 0)
	git := &cooldownMetadataGit{statusErrs: []error{context.DeadlineExceeded}, status: ports.GitStatus{RepoRoot: "/repo", Branch: "main", Modified: 1}}
	refresher := &metadataFakeRefresher{}
	svc := MetadataService{
		Store:                  store,
		Git:                    git,
		Refresher:              refresher,
		LockStore:              metadataDirectLock(store),
		CaptureFailureCooldown: time.Minute,
		Now:                    func() time.Time { return now },
	}
	sub := MetadataRepoSubscription{RepoRoot: "/repo", WorktreeRoot: "/repo", SessionNames: []string{"alpha"}}

	if _, err := svc.CaptureRepo(t.Context(), sub); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first CaptureRepo error = %v, want context deadline", err)
	}
	if got := git.calls.Load(); got != 1 {
		t.Fatalf("git status calls after first failure = %d, want 1", got)
	}
	changed, err := svc.CaptureRepo(t.Context(), sub)
	if err != nil || changed {
		t.Fatalf("cooldown CaptureRepo changed, err = %v, %v; want false, nil", changed, err)
	}
	if got := git.calls.Load(); got != 1 {
		t.Fatalf("git status calls during cooldown = %d, want still 1", got)
	}
	now = now.Add(time.Minute + time.Second)
	changed, err = svc.CaptureRepo(t.Context(), sub)
	if err != nil {
		t.Fatalf("CaptureRepo after cooldown error: %v", err)
	}
	if !changed || store.metadata("alpha").Modified != 1 || refresher.callCount() != 1 {
		t.Fatalf("after cooldown changed=%v metadata=%#v refresh=%d, want updated metadata and refresh", changed, store.metadata("alpha"), refresher.callCount())
	}
	if got := git.calls.Load(); got != 2 {
		t.Fatalf("git status calls after cooldown = %d, want 2", got)
	}
}

type cooldownMetadataGit struct {
	calls      atomic.Int64
	status     ports.GitStatus
	statusErrs []error
}

func (g *cooldownMetadataGit) RepoRoot(ctx context.Context, path string) (string, error) {
	return g.status.RepoRoot, nil
}

func (g *cooldownMetadataGit) Status(ctx context.Context, path string) (ports.GitStatus, error) {
	call := int(g.calls.Add(1)) - 1
	if call < len(g.statusErrs) && g.statusErrs[call] != nil {
		return ports.GitStatus{}, g.statusErrs[call]
	}
	return g.status, nil
}

func (g *cooldownMetadataGit) RepoInfo(ctx context.Context, path string) (ports.GitRepoInfo, error) {
	return ports.GitRepoInfo{RepoRoot: g.status.RepoRoot, WorktreeRoot: g.status.RepoRoot, Branch: g.status.Branch}, nil
}

func (g *cooldownMetadataGit) WatchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	return ports.GitWatchTargets{RepoRoot: g.status.RepoRoot, WorktreeRoot: g.status.RepoRoot}, nil
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

func TestMetadataServiceReconcileSkipsUnexpectedRepoInfoErrors(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}, paths: map[string]string{"alpha": "/bad", "beta": "/good"}}
	git := metadataFakeGit{
		infoErrs: map[string]error{"/bad": errors.New("boom")},
		infos:    map[string]ports.GitRepoInfo{"/good": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targets:  map[string]ports.GitWatchTargets{"/good": {RepoRoot: "/repo", WorktreeRoot: "/repo", Dirs: []string{"/repo"}}},
	}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, LockStore: metadataDirectLock(store)}

	subs, err := svc.Reconcile(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if got := subs["/repo"].SessionNames; len(got) != 1 || got[0] != "beta" {
		t.Fatalf("subscription sessions = %#v, want beta", got)
	}
}

func TestMetadataServiceReconcileSkipsUnexpectedWatchTargetErrors(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}}}
	tmux := metadataFakeTmux{sessions: []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}, paths: map[string]string{"alpha": "/bad", "beta": "/good"}}
	git := metadataFakeGit{
		infos:      map[string]ports.GitRepoInfo{"/bad": {RepoRoot: "/bad", WorktreeRoot: "/bad"}, "/good": {RepoRoot: "/repo", WorktreeRoot: "/repo", GitDir: "/repo/.git", CommonGitDir: "/repo/.git"}},
		targetErrs: map[string]error{"/bad": errors.New("boom")},
		targets:    map[string]ports.GitWatchTargets{"/good": {RepoRoot: "/repo", WorktreeRoot: "/repo", Dirs: []string{"/repo"}}},
	}
	svc := MetadataService{Store: store, Tmux: tmux, Git: git, LockStore: metadataDirectLock(store)}

	subs, err := svc.Reconcile(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if got := subs["/repo"].SessionNames; len(got) != 1 || got[0] != "beta" {
		t.Fatalf("subscription sessions = %#v, want beta", got)
	}
}
