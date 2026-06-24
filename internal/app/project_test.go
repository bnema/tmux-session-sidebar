package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type recordingFilesystem struct {
	resolved map[string]string
	dirs     map[string][]string
	calls    []string
}

func (f *recordingFilesystem) ResolvePath(path string) (string, error) {
	f.calls = append(f.calls, "resolve:"+path)
	resolved, ok := f.resolved[path]
	if !ok {
		return "", errors.New("unexpected root")
	}
	return resolved, nil
}

func (f *recordingFilesystem) ListImmediateDirs(root string) ([]string, error) {
	f.calls = append(f.calls, "list:"+root)
	return append([]string(nil), f.dirs[root]...), nil
}

func TestProjectCandidatesReturnMissingFilesystemDependency(t *testing.T) {
	restoreRunner := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		return "/projects", nil
	})
	defer restoreRunner()

	deps := runtimeDependencies()
	updatedDeps := deps
	updatedDeps.Filesystem = nil
	SetRuntimeDependencies(updatedDeps)
	t.Cleanup(func() { SetRuntimeDependencies(deps) })

	_, err := projectCandidates(context.Background())
	if !isMissingDependencyError(err) {
		t.Fatalf("projectCandidates error = %v, want missing filesystem dependency", err)
	}
}

func TestProjectCandidatesUseInjectedFilesystem(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/configured/root")
	restoreRunner := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" || !reflect.DeepEqual(args, []string{"show-options", "-gvq", "@session-sidebar-project-roots"}) {
			t.Fatalf("tmux call = %s %#v, want project roots option lookup", name, args)
		}
		return "$PROJECT_ROOT:/missing", nil
	})
	defer restoreRunner()

	deps := runtimeDependencies()
	fs := &recordingFilesystem{
		resolved: map[string]string{
			"/configured/root": "/real/root",
			"/missing":         "/real/missing",
		},
		dirs: map[string][]string{
			"/real/root": {"/real/root/alpha", "/real/root/beta"},
		},
	}
	updatedDeps := deps
	updatedDeps.Filesystem = fs
	SetRuntimeDependencies(updatedDeps)
	t.Cleanup(func() { SetRuntimeDependencies(deps) })

	got, err := projectCandidates(context.Background())
	if err != nil {
		t.Fatalf("projectCandidates returned error: %v", err)
	}
	gotPaths := []string{}
	for _, candidate := range got {
		gotPaths = append(gotPaths, candidate.Path)
	}
	wantPaths := []string{"/real/root/alpha", "/real/root/beta"}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("candidate paths = %#v, want %#v", gotPaths, wantPaths)
	}
	wantCalls := []string{"resolve:/configured/root", "list:/real/root", "resolve:/missing", "list:/real/missing"}
	if !reflect.DeepEqual(fs.calls, wantCalls) {
		t.Fatalf("filesystem calls = %#v, want %#v", fs.calls, wantCalls)
	}
}

var _ ports.FilesystemPort = (*recordingFilesystem)(nil)
