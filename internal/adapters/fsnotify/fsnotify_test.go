package fsnotify

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWatcherEmitsEventsForWatchedTree(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{root})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	assertWatchEvent(t, events, errs, path)
}

func TestWatcherAddsNewDirectoriesRecursively(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{root})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	dir := filepath.Join(root, "nested")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	assertWatchEvent(t, events, errs, dir)
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	assertWatchEvent(t, events, errs, path)
}

func TestWatcherSkipsExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	excluded := filepath.Join(root, "node_modules")
	if err := os.Mkdir(excluded, 0o755); err != nil {
		t.Fatalf("mkdir excluded: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{root})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	path := filepath.Join(excluded, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write excluded file: %v", err)
	}
	assertNoWatchEvent(t, events, errs, path)
}

func assertWatchEvent(t *testing.T, events <-chan Event, errs <-chan error, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Path == want {
				return
			}
		case err := <-errs:
			t.Fatalf("watch error: %v", err)
		case <-deadline:
			t.Fatalf("timed out waiting for event %q", want)
		}
	}
}

func assertNoWatchEvent(t *testing.T, events <-chan Event, errs <-chan error, path string) {
	t.Helper()
	deadline := time.After(250 * time.Millisecond)
	for {
		select {
		case event := <-events:
			if event.Path == path {
				t.Fatalf("unexpected event for excluded path %q", path)
			}
		case err := <-errs:
			t.Fatalf("watch error: %v", err)
		case <-deadline:
			return
		}
	}
}

func TestWatcherWatchesExplicitGitDirectories(t *testing.T) {
	root := t.TempDir()
	refs := filepath.Join(root, ".git", "refs")
	if err := os.MkdirAll(filepath.Join(refs, "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{root, refs})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	path := filepath.Join(refs, "heads", "main")
	if err := os.WriteFile(path, []byte("ref"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	assertWatchEvent(t, events, errs, path)
}

func TestWatcherAddsNewDirectoriesUnderExplicitGitRefs(t *testing.T) {
	root := t.TempDir()
	refs := filepath.Join(root, ".git", "refs")
	if err := os.MkdirAll(refs, 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{refs})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	remoteDir := filepath.Join(refs, "remotes", "origin")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatalf("mkdir remote refs: %v", err)
	}
	assertWatchEvent(t, events, errs, filepath.Join(refs, "remotes"))
	path := filepath.Join(remoteDir, "main")
	if err := os.WriteFile(path, []byte("ref"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	assertWatchEvent(t, events, errs, path)
}

func TestDefaultExcludesDoNotHideVendorOrTmp(t *testing.T) {
	excludes := (Watcher{}).excludes()
	for _, name := range []string{"vendor", "tmp"} {
		if slices.Contains(excludes, name) {
			t.Fatalf("default excludes contain %q; broad language/cache excludes can hide tracked repo changes: %#v", name, excludes)
		}
	}
}

func TestWatcherWatchesExplicitVendorAndTmpRootsRecursively(t *testing.T) {
	for _, name := range []string{"vendor", "tmp"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatalf("mkdir root: %v", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			events, errs, err := (Watcher{}).Watch(ctx, []string{root})
			if err != nil {
				t.Fatalf("Watch error: %v", err)
			}
			path := filepath.Join(root, "src", "file.txt")
			if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			assertWatchEvent(t, events, errs, path)
		})
	}
}

func TestWatcherDoesNotExcludeRootWithExcludedAncestorName(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "node_modules", "repo")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, errs, err := (Watcher{}).Watch(ctx, []string{root})
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	path := filepath.Join(root, "src", "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	assertWatchEvent(t, events, errs, path)
}
