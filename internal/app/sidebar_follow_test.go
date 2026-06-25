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

func TestSwitchClientDoesNotAdoptUnrelatedVisibleSidebar(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("seed sidebar state: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  switch-client) ;;
esac
`)

	tmuxPort := mocks.NewMockSidebarPort(t)

	if err := switchClient(ctx, "client-2", "beta", tmuxPort); err != nil {
		t.Fatalf("switchClient error: %v", err)
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState error: %v", err)
	}
	if state.VisibleClients["client-2"] || state.OwnerClient != "client-1" {
		t.Fatalf("state = %#v, want unrelated client not adopted", state)
	}
}

func TestSwitchClientMovesOwnerScopedSidebarAtomicallyBeforeSwitchingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true, "client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	base := mocks.NewMockSidebarPort(t)
	tmuxPort := &switchCapableSidebarPort{MockSidebarPort: base}
	var ops []string

	base.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	base.EXPECT().FindSidebarPane(ctx, "=beta:").Run(func(context.Context, string) {
		ops = append(ops, "find-target")
	}).Return(ports.PaneRef{WindowID: "@beta"}, nil)
	base.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-owner")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.switchFn = func(_ context.Context, clientID, sessionName, paneID, width string) error {
		if clientID != "client-1" || sessionName != "beta" || paneID != "%9" || width == "" {
			t.Fatalf("atomic switch args = %q %q %q %q", clientID, sessionName, paneID, width)
		}
		ops = append(ops, "move-switch-owner")
		return nil
	}

	if err := switchClient(ctx, "client-1", "beta", tmuxPort); err != nil {
		t.Fatalf("switchClient error: %v", err)
	}
	assertOps(t, ops, []string{"find-target", "find-owner", "move-switch-owner"})
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState error: %v", err)
	}
	if !state.VisibleClients["client-1"] || !state.VisibleClients["client-2"] {
		t.Fatalf("VisibleClients = %#v, want both clients preserved", state.VisibleClients)
	}
}

func TestSwitchClientToWindowWithExistingSidebarSwitchesWithoutAddingSidebar(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true, "client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	base := mocks.NewMockSidebarPort(t)
	var ops []string

	base.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	base.EXPECT().FindSidebarPane(ctx, "=beta:").Run(func(context.Context, string) {
		ops = append(ops, "find-target-sidebar")
	}).Return(ports.PaneRef{PaneID: "%22", WindowID: "@beta"}, nil)
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "switch-client\x00-c\x00client-1\x00-t\x00=beta:" {
			ops = append(ops, "switch-only")
			return "", nil
		}
		t.Fatalf("unexpected command args: %#v", args)
		return "", nil
	})
	defer restore()

	if err := switchClient(ctx, "client-1", "beta", base); err != nil {
		t.Fatalf("switchClient error: %v", err)
	}
	assertOps(t, ops, []string{"find-target-sidebar", "switch-only"})
}

func TestCreateProjectSwitchMovesOwnerScopedSidebarBeforeSwitchingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	base := mocks.NewMockSidebarPort(t)
	tmuxPort := &switchCapableSidebarPort{MockSidebarPort: base}
	var ops []string

	base.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	base.EXPECT().FindSidebarPane(ctx, "=beta:").Return(ports.PaneRef{WindowID: "@beta"}, nil)
	base.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Run(func(context.Context, string) {
		ops = append(ops, "find-owner")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.switchFn = func(_ context.Context, clientID, sessionName, paneID, _ string) error {
		if clientID != "client-1" || sessionName != "beta" || paneID != "%9" {
			t.Fatalf("atomic switch args = %q %q %q", clientID, sessionName, paneID)
		}
		ops = append(ops, "move-switch-owner")
		return nil
	}

	installFakeTmux(t, `#!/usr/bin/env bash
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
  *)
    printf 'unexpected tmux args: %s\n' "$*" >&2
    exit 1
    ;;
esac
`)

	if err := createProject(ctx, map[string]string{"client": "client-1", "project-path": "/work/beta"}, nil, tmuxPort); err != nil {
		t.Fatalf("createProject error: %v", err)
	}
	assertOps(t, ops, []string{"find-owner", "move-switch-owner"})
}

func TestSwitchClientRequiresAtomicSidebarSwitchBeforeEnsuringOwnerPane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmuxPort.EXPECT().FindSidebarPane(ctx, "=beta:").Return(ports.PaneRef{WindowID: "@beta"}, nil)

	err := switchClient(ctx, "client-1", "beta", tmuxPort)
	if err == nil || !strings.Contains(err.Error(), "requires atomic tmux move+switch support") {
		t.Fatalf("switchClient error = %v, want missing atomic switch support", err)
	}
}

func TestSwitchClientDoesNotSaveVisibilityIfAtomicSwitchFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	base := mocks.NewMockSidebarPort(t)
	tmuxPort := &switchCapableSidebarPort{MockSidebarPort: base}

	base.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	base.EXPECT().FindSidebarPane(ctx, "=beta:").Return(ports.PaneRef{WindowID: "@beta"}, nil)
	base.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.switchFn = func(context.Context, string, string, string, string) error {
		return errors.New("switch failed")
	}

	err := switchClient(ctx, "client-1", "beta", tmuxPort)
	if err == nil {
		t.Fatal("switchClient error = nil, want failure")
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState error: %v", err)
	}
	if !state.VisibleClients["client-1"] || state.VisibleClients["client-2"] || state.OwnerClient != "client-1" {
		t.Fatalf("state after failed switch = %#v, want unchanged", state)
	}
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
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{}, nil)
	tmuxPort.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, string, []string) {
		ops = append(ops, "ensure-owner")
	}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%9", mock.Anything).Run(func(context.Context, string, string, string, string) {
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
	assertOps(t, ops, []string{"ensure-owner", "attach-target"})
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
	tmuxPort.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{}, nil)
	tmuxPort.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, string, []string) {
		ops = append(ops, "ensure-owner")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmuxPort.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%10", mock.Anything).Run(func(context.Context, string, string, string, string) {
		ops = append(ops, "attach-target")
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@new"}, nil)

	err := withSidebarFollow(ctx, "client-1", tmuxPort, func() error {
		_, _ = tmux(ctx, "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	assertOps(t, ops, []string{"ensure-owner", "attach-target"})
}

func TestReconcileSidebarVisibilityClosesExistingPaneWhenConfiguredCloseAfterSwitch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmuxPort := mocks.NewMockSidebarPort(t)
	tmuxPort.EXPECT().CloseAfterSwitch(ctx).Return(true, nil)
	tmuxPort.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().ParkSidebarForClient(ctx, "client-1", "%9").Return(nil)

	if err := reconcileSidebarVisibilityForClient(ctx, "client-1", tmuxPort); err != nil {
		t.Fatalf("reconcileSidebarVisibilityForClient error: %v", err)
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState error: %v", err)
	}
	if state.Open || state.VisibleClients["client-1"] {
		t.Fatalf("state = %#v, want client-1 closed", state)
	}
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
	tmuxPort.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@old"}, nil)
	tmuxPort.EXPECT().ParkSidebarForClient(ctx, "client-1", "%9").Return(nil)

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

type switchCapableSidebarPort struct {
	*mocks.MockSidebarPort
	switchFn func(context.Context, string, string, string, string) error
}

func (s *switchCapableSidebarPort) AttachSidebarForClientAndSwitchClient(ctx context.Context, clientID string, sessionName string, paneID string, width string) error {
	if s.switchFn == nil {
		return errors.New("test switchCapableSidebarPort missing switchFn")
	}
	return s.switchFn(ctx, clientID, sessionName, paneID, width)
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
