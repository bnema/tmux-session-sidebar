package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/tmux-session-sidebar/adapters/daemonctl"
	"github.com/bnema/tmux-session-sidebar/adapters/ipcunix"
	"github.com/bnema/tmux-session-sidebar/internal/app"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type mainScopeProcess struct{ stdout string }

func (p mainScopeProcess) Exec(_ context.Context, _ string, _ []string) (ports.Result, error) {
	return ports.Result{Stdout: p.stdout}, nil
}

func TestBuildRuntimeIgnoresStaleDaemonFromPreviousTmuxServer(t *testing.T) {
	t.Cleanup(app.ResetRuntimeScopeForTest)
	stateHome := t.TempDir()
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", stateHome)
	oldScope := app.RuntimeScopeForProcess(t.Context(), mainScopeProcess{stdout: "/tmp/tmux/default\t111\n"})
	if err := os.MkdirAll(oldScope.Dir, 0o700); err != nil {
		t.Fatalf("mkdir old scope: %v", err)
	}
	if err := os.WriteFile(oldScope.IPCSocketPath, []byte("stale daemon placeholder"), 0o600); err != nil {
		t.Fatalf("write stale socket placeholder: %v", err)
	}

	router, currentScope := buildRuntimeRouter(t.Context(), mainScopeProcess{stdout: "/tmp/tmux/default\t222\n"})
	snapshot, ok := app.InspectRuntimeRouter(router)
	if !ok {
		t.Fatalf("router does not expose runtime snapshot: %T", router)
	}
	client := snapshot.IPCClient.(ipcunix.Client)
	if client.SocketPath == oldScope.IPCSocketPath {
		t.Fatalf("current client socket reused stale daemon socket %q", client.SocketPath)
	}
	if client.SocketPath != currentScope.IPCSocketPath {
		t.Fatalf("client socket = %q, want current scope socket %q", client.SocketPath, currentScope.IPCSocketPath)
	}
}

func TestBuildRuntimeUsesOneScopedDirForIPCAndDaemon(t *testing.T) {
	t.Cleanup(app.ResetRuntimeScopeForTest)
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	router, scope := buildRuntimeRouter(t.Context(), mainScopeProcess{stdout: "/tmp/tmux/default\t123\n"})

	got, ok := app.InspectRuntimeRouter(router)
	if !ok {
		t.Fatalf("router does not expose runtime snapshot: %T", router)
	}
	client, ok := got.IPCClient.(ipcunix.Client)
	if !ok {
		t.Fatalf("ipc client = %T", got.IPCClient)
	}
	server, ok := got.IPCServer.(ipcunix.Server)
	if !ok {
		t.Fatalf("ipc server = %T", got.IPCServer)
	}
	launcher, ok := got.DaemonLauncher.(daemonctl.Launcher)
	if !ok {
		t.Fatalf("daemon launcher = %T", got.DaemonLauncher)
	}
	if client.SocketPath != scope.IPCSocketPath || server.SocketPath != scope.IPCSocketPath || launcher.StateDir != scope.Dir {
		t.Fatalf("paths not composed from one scope: client=%q server=%q launcher=%q scope=%#v", client.SocketPath, server.SocketPath, launcher.StateDir, scope)
	}
	if filepath.Dir(client.SocketPath) != scope.Dir {
		t.Fatalf("socket dir = %q, want %q", filepath.Dir(client.SocketPath), scope.Dir)
	}
}
