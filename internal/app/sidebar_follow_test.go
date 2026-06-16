package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestSwitchClientAdoptsOpenSidebarForTargetClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("seed sidebar state: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  switch-client) ;;
esac
`)

	if err := switchClient(ctx, "client-2", "beta", nil); err != nil {
		t.Fatalf("switchClient error: %v", err)
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState error: %v", err)
	}
	if state.OwnerClient != "client-1" {
		t.Fatalf("OwnerClient = %q, want unchanged without sidebar port", state.OwnerClient)
	}

	tmuxPort := mocks.NewMockSidebarPort(t)
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSingletonSidebar(ctx).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "=gamma:", "%9", mock.Anything).Return(ports.PaneRef{PaneID: "%9", WindowID: "@3"}, nil)
	if err := switchClient(ctx, "client-2", "gamma", tmuxPort); err != nil {
		t.Fatalf("switchClient with sidebar error: %v", err)
	}
	state, err = persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState after switch error: %v", err)
	}
	if state.OwnerClient != "client-2" {
		t.Fatalf("OwnerClient = %q, want adopted target client", state.OwnerClient)
	}
}

func TestSwitchClientPrepositionsGlobalSidebarBeforeSwitchingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	var ops []string

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSingletonSidebar(ctx).Run(func(context.Context) {
		ops = append(ops, "find-singleton")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "=beta:", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@new"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "switch-client\x00-c\x00client-1\x00-t\x00=beta:" {
			ops = append(ops, "switch-client")
			return "", nil
		}
		t.Fatalf("unexpected command args: %#v", args)
		return "", nil
	})
	defer restore()

	if err := switchClient(ctx, "client-1", "beta", tmuxPort); err != nil {
		t.Fatalf("switchClient error: %v", err)
	}
	assertOps(t, ops, []string{"find-singleton", "attach-target", "switch-client"})
}

func TestCreateProjectSwitchPrepositionsGlobalSidebarBeforeSwitchingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	var ops []string
	var logPath string

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSingletonSidebar(ctx).Run(func(context.Context) {
		ops = append(ops, "find-singleton")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "=beta:", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		log, err := os.ReadFile(logPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read fake tmux log: %v", err)
		}
		if strings.Contains(string(log), "switch-client") {
			t.Fatal("sidebar attach happened after switch-client; want preposition before switching")
		}
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@new"}, nil)

	logPath = installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  list-sessions)
    printf '$2\tbeta\t1\t0\n'
    ;;
  show-options)
    case "${@: -1}" in
      @session-sidebar-width) printf '20\n' ;;
      *) printf '\n' ;;
    esac
    ;;
  switch-client)
    printf 'switch-client\n' >> "$TMUX_LOG"
    ;;
  *)
    printf 'unexpected tmux args: %s\n' "$*" >&2
    exit 1
    ;;
esac
`)

	if err := createProject(ctx, map[string]string{"client": "client-1", "project-path": "/work/beta"}, nil, tmuxPort); err != nil {
		t.Fatalf("createProject error: %v", err)
	}
	assertOps(t, ops, []string{"find-singleton", "attach-target"})
	if !strings.Contains(readLog(t, logPath), "switch-client") {
		t.Fatal("tmux switch-client was not called")
	}
}

func TestSwitchClientRestoresSidebarIfPrepositionedSwitchFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	var ops []string

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSingletonSidebar(ctx).Run(func(context.Context) {
		ops = append(ops, "find-singleton")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "=beta:", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@new"}, nil).Once()
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "rollback-attach")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "switch-client\x00-c\x00client-1\x00-t\x00=beta:" {
			ops = append(ops, "switch-client")
			return "no such session\n", errors.New("exit status 1")
		}
		t.Fatalf("unexpected command args: %#v", args)
		return "", nil
	})
	defer restore()

	err := switchClient(ctx, "client-1", "beta", tmuxPort)
	if err == nil {
		t.Fatal("switchClient error = nil, want failure")
	}
	assertOps(t, ops, []string{"find-singleton", "attach-target", "switch-client", "rollback-attach"})
}

func TestWithSidebarFollowPreservesGlobalSidebarWhenConfiguredToStayOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var calls [][]string
	var ops []string
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		calls = append(calls, append([]string(nil), args...))
		joined := strings.Join(args, "\x00")
		if joined == "switch-client\x00-c\x00client-1\x00-t\x00beta" {
			return "", nil
		}
		return "", nil
	})
	defer restore()

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, []string) {
		ops = append(ops, "ensure-singleton")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@new"}, nil)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		_, _ = tmux(ctx, "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	wantSubsequence := [][]string{
		{"switch-client", "-c", "client-1", "-t", "beta"},
	}
	assertCallSubsequence(t, calls, wantSubsequence)
	assertOps(t, ops, []string{"ensure-singleton", "attach-target"})
}

func TestWithSidebarFollowReopensGlobalSidebarWhenConfiguredToStayOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var ops []string
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	restore := stubCommandRunner(t, func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", nil
	})
	defer restore()

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, []string) {
		ops = append(ops, "ensure-singleton")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%10", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@new"}, nil)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		_, _ = tmux(ctx, "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	assertOps(t, ops, []string{"ensure-singleton", "attach-target"})
}

func TestWithSidebarFollowClosesOpenSidebarWhenConfiguredCloseAfterSwitch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var calls [][]string
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		return "", nil
	})
	defer restore()

	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(true, nil)
	tmuxPort.EXPECT().FindSingletonSidebar(ctx).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().ParkSingletonSidebar(ctx, "%9").Return(nil)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		_, _ = tmux(ctx, "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	assertCallSubsequence(t, calls, [][]string{
		{"switch-client", "-c", "client-1", "-t", "beta"},
	})
	for _, call := range calls {
		if len(call) > 0 && call[0] == "split-window" {
			t.Fatalf("split-window called despite close-after-switch=on: %#v", calls)
		}
	}
}

func TestWithSidebarFollowReturnsPersistedSidebarStateError(t *testing.T) {
	ctx := t.Context()
	stateHomeFile := filepath.Join(t.TempDir(), "state-home-file")
	if err := os.WriteFile(stateHomeFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateHomeFile)

	err := withSidebarFollow(ctx, "client-1", nil, func() error {
		t.Fatal("action should not run after sidebar state failure")
		return nil
	})
	if err == nil {
		t.Fatal("withSidebarFollow error = nil, want persisted sidebar state error")
	}
}

func TestWithSidebarFollowAllowsNilSidebarAfterActionWhenStateSaysFollow(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	called := false

	err := withSidebarFollow(ctx, "client-1", nil, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow error: %v", err)
	}
	if !called {
		t.Fatal("action was not called")
	}
}

func stubCommandRunner(t *testing.T, runner func(context.Context, string, ...string) (string, error)) func() {
	t.Helper()
	old := commandRunner
	commandRunner = runner
	return func() { commandRunner = old }
}

func assertCallSubsequence(t *testing.T, calls [][]string, want [][]string) {
	t.Helper()
	at := 0
	for _, call := range calls {
		if at < len(want) && reflect.DeepEqual(call, want[at]) {
			at++
		}
	}
	if at != len(want) {
		t.Fatalf("calls missing subsequence\nwant: %#v\ngot:  %#v", want, calls)
	}
}

func assertOps(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ops mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}
