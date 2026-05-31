package daemonctl

import (
	"context"
	"os"
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
