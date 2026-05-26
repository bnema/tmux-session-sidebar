package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestSwitchClientPrepositionsGlobalSidebarBeforeSwitchingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
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

func TestSwitchClientRestoresSidebarIfPrepositionedSwitchFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
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
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
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
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
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
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
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
