package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type scopeProcess struct{ stdout string }

func (p scopeProcess) Exec(_ context.Context, name string, args []string) (ports.Result, error) {
	if name != "tmux" {
		return ports.Result{}, nil
	}
	return ports.Result{Stdout: p.stdout}, nil
}

func TestRuntimeScopeUsesLegacyRootOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	scope := RuntimeScopeForProcess(t.Context(), nil)

	wantRoot := filepath.Join(stateHome, "tmux-session-sidebar")
	if scope.RootDir != wantRoot || scope.Dir != wantRoot || scope.StateDir != wantRoot || !scope.Legacy {
		t.Fatalf("scope = %#v, want legacy root/dir/state dir %q", scope, wantRoot)
	}
	if scope.IPCSocketPath != filepath.Join(wantRoot, "sidebar.sock") || scope.PIDPath != filepath.Join(wantRoot, "daemon.pid") || scope.ErrorsLogPath != filepath.Join(wantRoot, "errors.log") || scope.LocksDir != filepath.Join(wantRoot, "locks") {
		t.Fatalf("scope paths = %#v", scope)
	}
}

func TestRuntimeScopeIncludesSocketPathAndPID(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	a := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux socket/default\t111\n"})
	again := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux socket/default\t111\n"})
	b := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux socket/default\t222\n"})

	if a.Legacy || a.SocketPath != "/tmp/tmux socket/default" || a.ServerPID != "111" {
		t.Fatalf("scope identity = %#v", a)
	}
	if a.Dir != again.Dir {
		t.Fatalf("same socket and pid produced different dirs: %q vs %q", a.Dir, again.Dir)
	}
	if a.Dir == b.Dir {
		t.Fatalf("same socket with different pid produced same dir: %q", a.Dir)
	}
	if a.StateDir != again.StateDir {
		t.Fatalf("same socket and pid produced different state dirs: %q vs %q", a.StateDir, again.StateDir)
	}
	if a.StateDir != b.StateDir {
		t.Fatalf("same socket with different pid produced different state dirs: %q vs %q", a.StateDir, b.StateDir)
	}
	if !strings.HasPrefix(a.Dir, filepath.Join(stateHome, "tmux-session-sidebar", "servers")+string(filepath.Separator)) {
		t.Fatalf("scoped dir %q not under servers root", a.Dir)
	}
	if !strings.HasPrefix(a.StateDir, filepath.Join(stateHome, "tmux-session-sidebar", "state")+string(filepath.Separator)) {
		t.Fatalf("state dir %q not under state root", a.StateDir)
	}
}

func TestRuntimeScopeFallsBackToScopedTmuxEnvWhenIdentityQueryFails(t *testing.T) {
	tmuxEnv := "/tmp/tmux-fallback/default,777,0"
	t.Setenv("TMUX", tmuxEnv)
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "malformed\n"})
	if scope.Legacy {
		t.Fatalf("scope = %#v, want scoped fallback instead of legacy", scope)
	}
	if scope.SocketPath != "/tmp/tmux-fallback/default" || scope.ServerPID != "777" {
		t.Fatalf("scope identity = %#v, want identity from TMUX env", scope)
	}
	if !strings.HasPrefix(scope.Dir, filepath.Join(stateHome, "tmux-session-sidebar", "servers")+string(filepath.Separator)) {
		t.Fatalf("fallback dir = %q, want scoped server dir", scope.Dir)
	}
	if !strings.HasPrefix(scope.StateDir, filepath.Join(stateHome, "tmux-session-sidebar", "state")+string(filepath.Separator)) {
		t.Fatalf("fallback state dir = %q, want scoped state dir", scope.StateDir)
	}
}

func TestRuntimeScopeServerDirNameIsBoundedAndSafe(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", "/home/example/.local/state")
	unsafeSocket := "/tmp/" + strings.Repeat("../very long socket name with spaces 🔥", 20)

	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: unsafeSocket + "\t999\n"})
	name := filepath.Base(scope.Dir)

	if len(name) > 32 {
		t.Fatalf("scope dir name length = %d, want bounded <= 32 (%q)", len(name), name)
	}
	if len(scope.IPCSocketPath) > 107 {
		t.Fatalf("ipc socket path length = %d, want <= 107 for Unix sockets (%q)", len(scope.IPCSocketPath), scope.IPCSocketPath)
	}
	if want := boundedScopeHash(scope.IdentityKey); name != want {
		t.Fatalf("scope dir name = %q, want deterministic hash %q", name, want)
	}
}

func TestWriteRuntimeScopeMetadata(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t123\n"})

	if err := writeRuntimeScopeMetadata(scope); err != nil {
		t.Fatalf("writeRuntimeScopeMetadata() error = %v", err)
	}
	data, err := os.ReadFile(runtimeScopeMetadataPath(scope))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"legacy": false`, `"socketPath": "/tmp/tmux/default"`, `"serverPid": "123"`, `"identityKey":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("metadata %q missing %s", text, want)
		}
	}
}

func TestRuntimeScopeStillCurrentRejectsSameSocketDifferentPID(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	scope := RuntimeScopeForProcess(t.Context(), scopeProcess{stdout: "/tmp/tmux/default\t111\n"})
	oldRunner := commandRunner
	commandRunner = func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" || strings.Join(args, " ") != "display-message -p #{socket_path}\t#{pid}" {
			t.Fatalf("unexpected command %s %v", name, args)
		}
		return "/tmp/tmux/default\t222\n", nil
	}
	t.Cleanup(func() { commandRunner = oldRunner })

	current, err := runtimeScopeStillCurrent(t.Context(), scope)
	if err != nil {
		t.Fatalf("runtimeScopeStillCurrent() error = %v", err)
	}
	if current {
		t.Fatal("runtimeScopeStillCurrent() = true, want stale for same socket with different PID")
	}
}
