package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestSwitchClientMovesOpenSidebarAfterSwitchingSession(t *testing.T) {
	ctx := t.Context()
	tmuxPort := mocks.NewMockTmuxSidebarPort(t)
	var ops []string

	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-open-sidebar")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil).Once()
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-target")
	}).Return(ports.PaneRef{WindowID: "@new"}, nil).Once()
	tmuxPort.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%9", mock.Anything).Run(func(context.Context, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@new"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "switch-client\x00-c\x00client-1\x00-t\x00beta" {
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
	assertOps(t, ops, []string{"find-open-sidebar", "switch-client", "find-target", "attach-target"})
}

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
	assertOps(t, ops, []string{"find-old", "find-target", "attach-target"})
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

func matchesDaemonServeUICommand() func([]string) bool {
	return func(command []string) bool {
		return slices.Contains(command, "daemon") && slices.Contains(command, "serve-ui")
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
