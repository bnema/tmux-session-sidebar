package daemonctl

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type recordingProcess struct {
	name string
	args []string
}

func (p *recordingProcess) Exec(_ context.Context, name string, args []string) (ports.Result, error) {
	p.name = name
	p.args = append([]string(nil), args...)
	return ports.Result{}, nil
}

func TestLauncherEnsuresDaemonWithBackgroundTmuxCommand(t *testing.T) {
	process := &recordingProcess{}
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0o700); err != nil {
		t.Fatalf("chmod state dir: %v", err)
	}
	launcher := Launcher{Process: process, RuntimePath: "/tmp/runtime bin", StateDir: stateDir}

	if err := launcher.EnsureStarted(t.Context()); err != nil {
		t.Fatalf("EnsureStarted error: %v", err)
	}
	if process.name != "tmux" {
		t.Fatalf("process name = %q, want tmux", process.name)
	}
	wantCommand := "'/tmp/runtime bin' daemon serve >/dev/null 2>>'" + stateDir + "/errors.log'"
	wantArgs := []string{"run-shell", "-b", wantCommand}
	if !reflect.DeepEqual(process.args, wantArgs) {
		t.Fatalf("process args = %#v, want %#v", process.args, wantArgs)
	}
	if _, err := os.Stat(stateDir + "/errors.log"); !os.IsNotExist(err) {
		t.Fatalf("EnsureStarted created errors.log on the caller path; err=%v", err)
	}
}

func TestLauncherUsesPrivateChildDirWhenStateDirIsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	childDir := filepath.Join(tmpDir, ".tmux-session-sidebar-daemon")

	// Record original temp-root permissions so we can verify they aren't touched.
	origInfo, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	origPerms := origInfo.Mode().Perm()

	process := &recordingProcess{}
	launcher := Launcher{Process: process, RuntimePath: "/tmp/runtime"}

	if err := launcher.EnsureStarted(t.Context()); err != nil {
		t.Fatalf("EnsureStarted error: %v", err)
	}

	// Verify the temp root permissions are unchanged.
	afterInfo, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterInfo.Mode().Perm() != origPerms {
		t.Fatalf("temp root permissions changed: was %o, now %o", origPerms, afterInfo.Mode().Perm())
	}

	// Verify private child dir exists with 0o700.
	info, err := os.Stat(childDir)
	if err != nil {
		t.Fatalf("private child dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("private child dir is not a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("private child dir permissions = %o, want 0700", info.Mode().Perm())
	}
}
