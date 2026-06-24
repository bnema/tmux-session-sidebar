package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestMetadataCaptureEngineCapturesThroughFocusedDependency(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}}}
	svc := MetadataService{Store: store, Query: tmux, Git: git, LockStore: metadataDirectLock(store), GitStatusTimeout: time.Second, GitStatusConcurrency: 1}

	changed, err := newMetadataCaptureEngine(&svc).Capture(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true}, nil)
	if err != nil {
		t.Fatalf("Capture error: %v", err)
	}
	if !changed {
		t.Fatal("Capture changed = false, want true")
	}
	if got := store.metadata("alpha"); got.Branch != "main" || got.Modified != 1 {
		t.Fatalf("metadata = %#v, want captured git status", got)
	}
}

func TestMetadataSubscriptionReconcilerBuildsThroughFocusedDependency(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{"alpha": {Kind: "project", ProjectPath: "/repo"}}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		infos:   map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo"}},
		targets: map[string]ports.GitWatchTargets{"/repo": {Files: []string{"/repo/.git/HEAD"}, Dirs: []string{"/repo/.git/refs"}}},
	}
	svc := MetadataService{Store: store, Query: tmux, Git: git}

	subs, err := newMetadataSubscriptionReconciler(&svc).Reconcile(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	sub := subs["/repo"]
	if sub.RepoRoot != "/repo" || sub.WorktreeRoot != "/repo" || len(sub.SessionNames) != 1 || sub.SessionNames[0] != "alpha" {
		t.Fatalf("subscription = %#v, want repo subscription for alpha", sub)
	}
}

func TestMetadataWatchCoordinatorFallsBackWithoutWatcher(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}},
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {Files: []string{"/repo/.git/HEAD"}}},
	}
	svc := MetadataService{Store: store, Query: tmux, Git: git, LockStore: metadataDirectLock(store), GitStatusTimeout: time.Second, GitStatusConcurrency: 1}

	err := newMetadataWatchCoordinator(&svc).runWatchOnce(t.Context(), ports.ConfigSnapshot{MetadataSublineEnabled: true})
	if !errors.Is(err, errMetadataFallback) {
		t.Fatalf("runWatchOnce error = %v, want metadata fallback", err)
	}
	if got := store.metadata("alpha"); got.Branch != "main" {
		t.Fatalf("metadata = %#v, want initial capture before fallback", got)
	}
}

func TestMetadataWatchCoordinatorIgnoresNilWatcherErrors(t *testing.T) {
	store := &metadataFakeStore{state: ports.PersistedState{Metadata: map[string]ports.GitStatus{}}}
	tmux := metadataFakeTmux{sessions: []ports.SessionSnapshot{{Name: "alpha"}}, paths: map[string]string{"alpha": "/repo"}}
	git := metadataFakeGit{
		statuses: map[string]ports.GitStatus{"/repo": {RepoRoot: "/repo", Branch: "main", Modified: 1}},
		infos:    map[string]ports.GitRepoInfo{"/repo": {RepoRoot: "/repo", WorktreeRoot: "/repo"}},
		targets:  map[string]ports.GitWatchTargets{"/repo": {Files: []string{"/repo/.git/HEAD"}}},
	}
	watcher := newMetadataFakeWatcher()
	svc := MetadataService{Store: store, Query: tmux, Git: git, Watcher: watcher, LockStore: metadataDirectLock(store), GitStatusTimeout: time.Second, GitStatusConcurrency: 1, ReconcileInterval: time.Hour}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- newMetadataWatchCoordinator(&svc).runWatchOnce(ctx, ports.ConfigSnapshot{MetadataSublineEnabled: true})
	}()
	watcher.waitStarted(t)

	watcher.errs <- nil
	select {
	case err := <-done:
		t.Fatalf("runWatchOnce returned after nil watcher error: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runWatchOnce error = %v, want context canceled", err)
	}
}
