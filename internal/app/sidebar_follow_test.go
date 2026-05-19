package app

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestWithSidebarFollowPreservesOldSidebarAndOpensTargetWhenConfiguredToStayOpen(t *testing.T) {
	var calls [][]string
	var ops []string
	ctx := t.Context()
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

	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-old")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-target")
	}).Return(ports.PaneRef{WindowID: "@new"}, nil).Once()
	tmuxPort.EXPECT().OpenSidebar(ctx, "client-1", mock.MatchedBy(matchesUIRunCommand("client-1"))).Run(func(context.Context, string, []string) {
		ops = append(ops, "open-new")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@new"}, nil)

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
	assertOps(t, ops, []string{"find-old", "find-target", "open-new"})
}

func TestWithSidebarFollowReusesExistingTargetSidebarWhenConfiguredToStayOpen(t *testing.T) {
	var ops []string
	ctx := t.Context()
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
	restore := stubCommandRunner(t, func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", nil
	})
	defer restore()

	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-old")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-target")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@new"}, nil).Once()
	tmuxPort.EXPECT().RefreshSidebar(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "refresh-target")
	}).Return(nil)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		_, _ = tmux(ctx, "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	assertOps(t, ops, []string{"find-old", "find-target", "refresh-target"})
}

func TestWithSidebarFollowClosesOpenSidebarWhenConfiguredCloseAfterSwitch(t *testing.T) {
	var calls [][]string
	ctx := t.Context()
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		return "", nil
	})
	defer restore()

	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(true, nil)
	tmuxPort.EXPECT().CloseSidebarPane(ctx, "%9").Return(nil)

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

func TestWithSidebarFollowReturnsSidebarDiscoveryError(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{}, boom)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		t.Fatal("action should not run after sidebar discovery failure")
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("withSidebarFollow error = %v, want %v", err, boom)
	}
}

func matchesUIRunCommand(client string) func([]string) bool {
	return func(command []string) bool {
		if !slices.Contains(command, "ui") || !slices.Contains(command, "run") {
			return false
		}
		for i, arg := range command {
			if arg == "--client" {
				return i+1 < len(command) && command[i+1] == client
			}
		}
		return false
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

func testExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}
