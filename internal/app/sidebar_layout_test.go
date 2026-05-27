package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestOpenSidebarAttachesSingletonPaneToClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	tmux := mocks.NewMockTmuxSidebarPort(t)

	tmux.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, []string) {
		state, err := loadSidebarState(ctx)
		if err != nil {
			t.Fatalf("loadSidebarState during ensure: %v", err)
		}
		if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-1" {
			t.Fatalf("sidebar state during ensure = %#v, want already open for client-1", state.Sidebar)
		}
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%10", "20").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

	if err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux); err != nil {
		t.Fatalf("openSidebar returned error: %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-1" {
		t.Fatalf("sidebar state = %#v, want open for client-1", state.Sidebar)
	}
}

func TestOpenSidebarRollsBackVisibilityWhenEnsureFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	boom := errors.New("ensure failed")
	tmux := mocks.NewMockTmuxSidebarPort(t)
	tmux.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || state.Sidebar.Open || state.Sidebar.OwnerClient != "" {
		t.Fatalf("sidebar state after ensure failure = %#v, want closed", state.Sidebar)
	}
}

func TestOpenSidebarRollsBackVisibilityWhenAttachFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	boom := errors.New("attach failed")
	tmux := mocks.NewMockTmuxSidebarPort(t)
	tmux.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%10", "20").Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || state.Sidebar.Open || state.Sidebar.OwnerClient != "" {
		t.Fatalf("sidebar state after attach failure = %#v, want closed", state.Sidebar)
	}
}

func TestCloseSidebarParksGlobalSingletonPane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockTmuxSidebarPort(t)

	tmux.EXPECT().FindSingletonSidebar(ctx).Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)
	tmux.EXPECT().ParkSingletonSidebar(ctx, "%10").Return(nil)

	if err := closeSidebar(ctx, tmux); err != nil {
		t.Fatalf("closeSidebar returned error: %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || state.Sidebar.Open || state.Sidebar.OwnerClient != "" {
		t.Fatalf("sidebar state = %#v, want closed", state.Sidebar)
	}
}

func TestHookClientSessionChangedSkipsSidebarReconcileForInternalHookSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	installFakeTmux(t, fakeHookCaptureTmuxScript())
	sidebar := mocks.NewMockTmuxSidebarPort(t)

	err := (runtimeRouter{sidebar: sidebar}).Handle(ctx, Route{
		Path:  "hook/client-session-changed",
		Flags: map[string]string{"client": "popup-client", "session": "__floating-popup-1"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("hook client-session-changed error: %v", err)
	}
}

func TestHookClientDetachedDoesNotReconcileSidebarForDetachingClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "popup-client"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	installFakeTmux(t, fakeHookCaptureTmuxScript())
	sidebar := mocks.NewMockTmuxSidebarPort(t)

	err := (runtimeRouter{sidebar: sidebar}).Handle(ctx, Route{
		Path:  "hook/client-detached",
		Flags: map[string]string{"client": "popup-client", "session": "alpha"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("hook client-detached error: %v", err)
	}
}

func fakeHookCaptureTmuxScript() string {
	return `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-heat-recent-hours) printf '1\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t1\n$2\t__floating-popup-1\t1\t1\n' ;;
  list-clients) printf 'popup-client\t$2\t@2\t%2\t__floating-popup-1\n' ;;
  display-message) printf '%s\n' "$PWD" ;;
  list-panes) ;;
  *) ;;
esac
`
}

func TestReconcileSidebarVisibilityForClientReopensGlobalSidebarForOwnerClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-heat-recent-hours) printf '1\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  *) ;;
esac
`)
	tmux := mocks.NewMockTmuxSidebarPort(t)
	tmux.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmux.EXPECT().EnsureSingletonSidebar(ctx, mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSingletonSidebar(ctx, "client-1", "%10", "20").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

	if err := reconcileSidebarVisibilityForClient(ctx, "client-1", tmux); err != nil {
		t.Fatalf("reconcileSidebarVisibilityForClient error: %v", err)
	}
}

func TestRuntimeRouterProxiesSidebarRoutesThroughIPCClient(t *testing.T) {
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	ipc.EXPECT().Send(ctx, ports.SidebarToggleRequest("client-1")).Return(ports.Response{OK: true}, nil)

	router := NewRuntimeRouter(nil, ipc, nil)
	if err := router.Handle(ctx, Route{Path: "sidebar/toggle", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle sidebar/toggle error: %v", err)
	}
}

func TestRuntimeRouterReturnsAmbiguousIPCErrorWithoutDirectFallback(t *testing.T) {
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	timeoutErr := errors.New("daemon timeout")
	ipc.EXPECT().Send(ctx, ports.SidebarCloseRequest("client-1")).Return(ports.Response{}, timeoutErr)

	router := NewRuntimeRouter(nil, ipc, nil)
	err := router.Handle(ctx, Route{Path: "sidebar/close", Flags: map[string]string{"client": "client-1"}}, nil, nil)
	if !errors.Is(err, timeoutErr) {
		t.Fatalf("Handle sidebar/close error = %v, want timeout error", err)
	}
	if err != nil && strings.Contains(err.Error(), "sidebar port is required") {
		t.Fatalf("Handle sidebar/close error = %v, want no direct fallback", err)
	}
}

func TestRuntimeRouterUsesDirectFallbackForUnavailableIPCSocket(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	tmux := mocks.NewMockTmuxSidebarPort(t)
	ipc.EXPECT().Send(ctx, ports.SidebarCloseRequest("client-1")).Return(ports.Response{}, ports.ErrIPCConnectionRefused)
	tmux.EXPECT().FindSingletonSidebar(ctx).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
	tmux.EXPECT().ParkSingletonSidebar(ctx, "%9").Return(nil)

	router := NewRuntimeRouter(tmux, ipc, nil)
	if err := router.Handle(ctx, Route{Path: "sidebar/close", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle sidebar/close error: %v", err)
	}
}

func TestRuntimeRouterProxiesSidebarOpenWidthThroughIPCClient(t *testing.T) {
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	ipc.EXPECT().Send(ctx, ports.SidebarOpenRequest("client-1", "30")).Return(ports.Response{OK: true}, nil)

	router := NewRuntimeRouter(nil, ipc, nil)
	if err := router.Handle(ctx, Route{Path: "sidebar/open", Flags: map[string]string{"client": "client-1", "width": "30"}}, nil, nil); err != nil {
		t.Fatalf("Handle sidebar/open error: %v", err)
	}
}

func TestRuntimeRouterRequiresSidebarPortForSidebarRoutes(t *testing.T) {
	err := (runtimeRouter{}).Handle(t.Context(), Route{Path: "sidebar/toggle"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sidebar port is required") {
		t.Fatalf("Handle error = %v, want missing sidebar port error", err)
	}
}

func TestRuntimeRouterAllowsNonSidebarRoutesWithoutSidebarPort(t *testing.T) {
	if err := (runtimeRouter{}).Handle(t.Context(), Route{Path: "hook/client-resized"}, nil, nil); err != nil {
		t.Fatalf("Handle non-sidebar route error: %v", err)
	}
}

func TestResizeHooksWithoutTargetAreNoops(t *testing.T) {
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		t.Fatalf("unexpected command %s %#v", name, args)
		return "", nil
	})
	defer restore()

	for _, path := range []string{"hook/window-resized", "hook/client-resized"} {
		if err := (runtimeRouter{}).Handle(t.Context(), Route{Path: path}, nil, nil); err != nil {
			t.Fatalf("Handle(%s) error: %v", path, err)
		}
	}
}

func TestWindowResizedHookResizesProvidedSidebarPaneToConfiguredWidth(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "30\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookUsesWindowToFindMarkedSidebarPane(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "list-panes\x00-t\x00@1\x00-f\x00#{==:#{@session-sidebar-pane},1}\x00-F\x00#{pane_id}":
			return "%9\n", nil
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "30\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestClientResizedHookUsesClientToFindMarkedSidebarPane(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00-t\x00client-1\x00#{window_id}":
			return "@1\n", nil
		case "list-panes\x00-t\x00@1\x00-f\x00#{==:#{@session-sidebar-pane},1}\x00-F\x00#{pane_id}":
			return "%9\n", nil
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "30\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/client-resized", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookUsesFirstMarkedSidebarPaneWhenMultipleAreReturned(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "list-panes\x00-t\x00@1\x00-f\x00#{==:#{@session-sidebar-pane},1}\x00-F\x00#{pane_id}":
			return "%9\n%10\n", nil
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "30\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected command %s %#v", name, args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookDoesNothingWhenSidebarIsMissing(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "list-panes\x00-t\x00@1\x00-f\x00#{==:#{@session-sidebar-pane},1}\x00-F\x00#{pane_id}":
			return "", nil
		default:
			t.Fatalf("unexpected command %s %#v", name, args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookIgnoresMissingWindowTarget(t *testing.T) {
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "list-panes\x00-t\x00@1\x00-f\x00#{==:#{@session-sidebar-pane},1}\x00-F\x00#{pane_id}" {
			return "can't find window: @1\n", errors.New("exit status 1")
		}
		t.Fatalf("unexpected command %s %#v", name, args)
		return "", nil
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(t.Context(), Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestClientResizedHookIgnoresMissingClientTarget(t *testing.T) {
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		if strings.Join(args, "\x00") == "display-message\x00-p\x00-t\x00client-1\x00#{window_id}" {
			return "can't find client: client-1\n", errors.New("exit status 1")
		}
		t.Fatalf("unexpected command %s %#v", name, args)
		return "", nil
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(t.Context(), Route{Path: "hook/client-resized", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookIgnoresSidebarPaneThatDisappearsBeforeResize(t *testing.T) {
	ctx := t.Context()

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "30\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "can't find pane: %9\n", errors.New("exit status 1")
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestScheduleSidebarLayoutRestoreOnExitUsesProvidedPane(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockTmuxSidebarPort(t)

	tmux.EXPECT().ScheduleSidebarRestoreOnExit(ctx, "", "%9").Return(nil)

	scheduleSidebarLayoutRestoreOnExit(ctx, map[string]string{"pane": "%9"}, tmux)
}

func TestScheduleSidebarLayoutRestoreOnExitUsesTmuxPaneFallback(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockTmuxSidebarPort(t)
	t.Setenv("TMUX_PANE", "%8")

	tmux.EXPECT().ScheduleSidebarRestoreOnExit(ctx, "client-1", "%8").Return(nil)

	scheduleSidebarLayoutRestoreOnExit(ctx, map[string]string{"client": "client-1"}, tmux)
}
