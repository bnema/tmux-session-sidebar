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

type resizeSyncSidebarPort struct {
	*mocks.MockSidebarPort
	syncCalls    []string
	captureCalls []string
}

func (d *resizeSyncSidebarPort) CaptureAttachedSidebarWidthBaseline(_ context.Context, windowID string, paneID string, width string, options ports.SidebarResizeOptions) error {
	d.captureCalls = append(d.captureCalls, windowID+"|"+paneID+"|"+width)
	if options.Logger != nil {
		options.Logger.Debug("resize-capture-port", []ports.LogField{{Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
	}
	return nil
}

func (d *resizeSyncSidebarPort) SyncAttachedSidebarWidth(_ context.Context, windowID string, paneID string, width string, options ports.SidebarResizeOptions) error {
	d.syncCalls = append(d.syncCalls, windowID+"|"+paneID+"|"+width)
	if options.Logger != nil {
		options.Logger.Debug("resize-sync-port", []ports.LogField{{Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
	}
	return nil
}

func TestOpenSidebarAttachesOwnerScopedPaneToClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)

	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Run(func(context.Context, string, []string) {
		state, err := loadSidebarState(ctx)
		if err != nil {
			t.Fatalf("loadSidebarState during ensure: %v", err)
		}
		if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-1" {
			t.Fatalf("sidebar state during ensure = %#v, want already open for client-1", state.Sidebar)
		}
	}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%10", "20").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

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
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar != nil && (state.Sidebar.Open || state.Sidebar.OwnerClient != "" || len(state.Sidebar.VisibleClients) != 0) {
		t.Fatalf("sidebar state after ensure failure = %#v, want absent or closed", state.Sidebar)
	}
}

func TestOpenSidebarRestoresPreExistingVisibilityWhenEnsureFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	boom := errors.New("ensure failed")
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-2", VisibleClients: map[string]bool{"client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-2" || state.Sidebar.VisibleClients["client-1"] || !state.Sidebar.VisibleClients["client-2"] {
		t.Fatalf("sidebar state after ensure failure = %#v, want original client-2 visibility", state.Sidebar)
	}
}

func TestOpenSidebarRollsBackVisibilityWhenAttachFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	boom := errors.New("attach failed")
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%10", "20").Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar != nil && (state.Sidebar.Open || state.Sidebar.OwnerClient != "" || len(state.Sidebar.VisibleClients) != 0) {
		t.Fatalf("sidebar state after attach failure = %#v, want absent or closed", state.Sidebar)
	}
}

func TestOpenSidebarRestoresPreExistingVisibilityWhenAttachFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	boom := errors.New("attach failed")
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-2", VisibleClients: map[string]bool{"client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%10", "20").Return(ports.PaneRef{}, boom)

	err := openSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux)
	if !errors.Is(err, boom) {
		t.Fatalf("openSidebar error = %v, want %v", err, boom)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-2" || state.Sidebar.VisibleClients["client-1"] || !state.Sidebar.VisibleClients["client-2"] {
		t.Fatalf("sidebar state after attach failure = %#v, want original client-2 visibility", state.Sidebar)
	}
}

func TestToggleSidebarOpensWhenPersistedOpenStateHasNoVisiblePane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)

	tmux.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{WindowID: "@1"}, nil)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSidebarForClient(ctx, "client-1", "@1", "%10", "20").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

	if err := toggleSidebar(ctx, map[string]string{"client": "client-1", "width": "20"}, tmux); err != nil {
		t.Fatalf("toggleSidebar returned error: %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-1" {
		t.Fatalf("sidebar state = %#v, want open for client-1", state.Sidebar)
	}
}

func TestCloseSidebarWithoutClientParksAllOwnerScopedPanes(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true, "client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)

	tmux.EXPECT().ParkAllSidebars(ctx).Return(nil)

	if err := closeSidebar(ctx, "", tmux); err != nil {
		t.Fatalf("closeSidebar returned error: %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if state.Sidebar == nil || state.Sidebar.Open || state.Sidebar.OwnerClient != "" || len(state.Sidebar.VisibleClients) != 0 {
		t.Fatalf("sidebar state = %#v, want closed with no visible clients", state.Sidebar)
	}
}

func TestCloseSidebarForClientParksOnlyThatClientAndPreservesOtherVisibleClients(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true, "client-2": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)
	tmux.EXPECT().ParkSidebarForClient(ctx, "client-1", "%10").Return(nil)

	if err := closeSidebar(ctx, "client-1", tmux); err != nil {
		t.Fatalf("closeSidebar returned error: %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if !state.Sidebar.Open || state.Sidebar.VisibleClients["client-1"] || !state.Sidebar.VisibleClients["client-2"] {
		t.Fatalf("sidebar state after client close = %#v, want only client-2 visible", state.Sidebar)
	}
}

func TestAdoptPersistedOpenSidebarIgnoresNonVisibleNonOwnerClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "other", VisibleClients: map[string]bool{"other": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)

	if err := adoptPersistedOpenSidebar(ctx, "client-1", tmux); err != nil {
		t.Fatalf("adoptPersistedOpenSidebar error: %v", err)
	}
}

func TestReconcileSidebarVisibilityForClientIsIdempotentWhenPaneExists(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1", VisibleClients: map[string]bool{"client-1": true}}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().CloseAfterSwitch(ctx).Return(false, nil).Twice()
	tmux.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil).Twice()

	for i := 0; i < 2; i++ {
		if err := reconcileSidebarVisibilityForClient(ctx, "client-1", tmux); err != nil {
			t.Fatalf("reconcileSidebarVisibilityForClient #%d error: %v", i+1, err)
		}
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
	sidebar := mocks.NewMockSidebarPort(t)

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
	sidebar := mocks.NewMockSidebarPort(t)

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
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
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
    if [ "$2" = "-g" ] && [ $# -eq 2 ]; then
      printf '@session-sidebar-key M-b\n'
      printf '@session-sidebar-width 30\n'
      printf '@session-sidebar-project-roots \n'
      printf '@session-sidebar-close-after-switch off\n'
      printf '@session-sidebar-heat-colors on\n'
      printf '@session-sidebar-heat-half-life-hours 8\n'
      printf '@session-sidebar-heat-stale-hours 24\n'
      printf '@session-sidebar-heat-refresh-seconds 60\n'
      printf '@session-sidebar-heat-recent 1h\n'
      printf '@session-sidebar-heat-max-highlighted 0\n'
      printf '@session-sidebar-activity-debug-log off\n'
      printf '@session-sidebar-agent-attention on\n'
      printf '@session-sidebar-agent-attention-animation pulse\n'
      printf '@session-sidebar-auto-sort-recent off\n'
      printf '@session-sidebar-restore-sessions auto\n'
      printf '@session-sidebar-continuum-grace-seconds 0\n'
      printf '@session-sidebar-metadata-subline off\n'
      printf '@session-sidebar-metadata-inactive off\n'
    else
      case "$3" in
        @session-sidebar-key) printf 'M-b\n' ;;
        @session-sidebar-width) printf '30\n' ;;
        @session-sidebar-project-roots) printf '\n' ;;
        @session-sidebar-close-after-switch) printf 'off\n' ;;
        @session-sidebar-heat-colors) printf 'on\n' ;;
        @session-sidebar-heat-half-life-hours) printf '8\n' ;;
        @session-sidebar-heat-stale-hours) printf '24\n' ;;
        @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
        @session-sidebar-heat-recent) printf '1h\n' ;;
        @session-sidebar-heat-max-highlighted) printf '0\n' ;;
        @session-sidebar-activity-debug-log) printf 'off\n' ;;
        @session-sidebar-agent-attention) printf 'on\n' ;;
        *) printf '\n' ;;
      esac
    fi ;;
  *) ;;
esac
`)
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{}, nil)
	tmux.EXPECT().CloseAfterSwitch(ctx).Return(false, nil)
	tmux.EXPECT().EnsureSidebarForClient(ctx, "client-1", mock.MatchedBy(matchesDaemonServeUICommand())).Return(ports.PaneRef{PaneID: "%10", WindowID: "@hidden"}, nil)
	tmux.EXPECT().AttachSidebarForClient(ctx, "client-1", "client-1", "%10", "30").Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

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
	tmux := mocks.NewMockSidebarPort(t)
	ipc.EXPECT().Send(ctx, ports.SidebarCloseRequest("client-1")).Return(ports.Response{}, ports.ErrIPCConnectionRefused)
	tmux.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
	tmux.EXPECT().ParkSidebarForClient(ctx, "client-1", "%9").Return(nil)

	router := NewRuntimeRouter(tmux, ipc, nil)
	if err := router.Handle(ctx, Route{Path: "sidebar/close", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle sidebar/close error: %v", err)
	}
}

func TestRuntimeRouterUsesDirectFallbackForStaleScopedIPC(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	tmux := mocks.NewMockSidebarPort(t)
	ipc.EXPECT().Send(ctx, ports.SidebarCloseRequest("client-1")).Return(ports.Response{OK: false, Message: "daemon tmux server identity is stale", ErrorCode: ports.IPCErrorStaleScope}, nil)
	tmux.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
	tmux.EXPECT().ParkSidebarForClient(ctx, "client-1", "%9").Return(nil)

	router := NewRuntimeRouter(tmux, ipc, nil)
	if err := router.Handle(ctx, Route{Path: "sidebar/close", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle sidebar/close error: %v", err)
	}
}

func TestRuntimeRouterEnsuresDaemonBeforeDirectFallbackForUnavailableIPC(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := t.Context()
	ipc := mocks.NewMockIPCClientPort(t)
	tmux := mocks.NewMockSidebarPort(t)
	daemon := mocks.NewMockDaemonLauncherPort(t)
	mock.InOrder(
		ipc.EXPECT().Send(ctx, ports.SidebarCloseRequest("client-1")).Return(ports.Response{}, ports.ErrIPCSocketMissing).Call,
		daemon.EXPECT().EnsureStarted(ctx).Return(nil).Call,
		tmux.EXPECT().FindSidebarPaneForClient(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil).Call,
		tmux.EXPECT().ParkSidebarForClient(ctx, "client-1", "%9").Return(nil).Call,
	)

	router := NewRuntimeRouterWithDaemon(tmux, ipc, nil, daemon)
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

	for _, path := range []string{"hook/window-resized", "hook/client-resized", "hook/window-layout-changed"} {
		if err := (runtimeRouter{}).Handle(t.Context(), Route{Path: path}, nil, nil); err != nil {
			t.Fatalf("Handle(%s) error: %v", path, err)
		}
	}
}

func TestWindowResizedHookResizesProvidedSidebarPaneToConfiguredWidth(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookLogsResizeDiagnosticsWhenDebugEnabled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\ton\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	log := string(content)
	for _, want := range []string{"debug: resize-hook", "path=hook/window-resized", "target=%9", "pane=%9", "window=@1", "width=30", "handler=resize-pane"} {
		if !strings.Contains(log, want) {
			t.Fatalf("activity log missing %q: %q", want, log)
		}
	}
}

func TestWindowResizedHookDoesNotCreateActivityLogWhenDebugDisabled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activity log stat error = %v, want not exist", err)
	}
}

func TestWindowResizedHookContinuesWhenDebugLogCannotOpen(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(stateFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateFile)
	t.Setenv("TMUX", "")
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\ton\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookPassesDebugLoggerToSidebarPort(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	ctx := t.Context()
	tmux := &resizeSyncSidebarPort{MockSidebarPort: mocks.NewMockSidebarPort(t)}
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\ton\n", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	log := string(content)
	for _, want := range []string{"debug: resize-sync-port", "window=@1", "pane=%9", "width=30"} {
		if !strings.Contains(log, want) {
			t.Fatalf("activity log missing %q: %q", want, log)
		}
	}
}

func TestClientResizedHookUsesClientToFindMarkedSidebarPane(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/client-resized", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookDoesNothingWhenSidebarIsMissing(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "@1").Return(ports.PaneRef{WindowID: "@1"}, nil)

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowLayoutChangedHookDelegatesToSidebarBaselineCaptureWhenAvailable(t *testing.T) {
	ctx := t.Context()
	tmux := &resizeSyncSidebarPort{MockSidebarPort: mocks.NewMockSidebarPort(t)}
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-layout-changed", Flags: map[string]string{"window": "@1", "pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if got := tmux.captureCalls; !reflect.DeepEqual(got, []string{"@1|%9|30"}) {
		t.Fatalf("capture calls = %#v, want [@1|%%9|30]", got)
	}
}

func TestWindowResizedHookDelegatesToSidebarResizeSyncWhenAvailable(t *testing.T) {
	ctx := t.Context()
	tmux := &resizeSyncSidebarPort{MockSidebarPort: mocks.NewMockSidebarPort(t)}
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1", "pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if got := tmux.syncCalls; !reflect.DeepEqual(got, []string{"@1|%9|30"}) {
		t.Fatalf("sync calls = %#v, want [@1|%%9|30]", got)
	}
}

func TestWindowResizedHookIgnoresMissingWindowTarget(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "@1").Return(ports.PaneRef{}, ports.ErrMultiplexerTargetGone)

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"window": "@1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestClientResizedHookIgnoresMissingClientTarget(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "client-1").Return(ports.PaneRef{}, ports.ErrMultiplexerTargetGone)

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/client-resized", Flags: map[string]string{"client": "client-1"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestWindowResizedHookIgnoresSidebarPaneThatDisappearsBeforeResize(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	tmux.EXPECT().FindSidebarPane(ctx, "%9").Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)

	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00#{@session-sidebar-width}\t#{@session-sidebar-activity-debug-log}":
			return "30\toff\n", nil
		case "resize-pane\x00-t\x00%9\x00-x\x0030":
			return "can't find pane: %9\n", errors.New("exit status 1")
		default:
			t.Fatalf("unexpected tmux args: %#v", args)
			return "", nil
		}
	})
	defer restore()

	if err := (runtimeRouter{sidebar: tmux}).Handle(ctx, Route{Path: "hook/window-resized", Flags: map[string]string{"pane": "%9"}}, nil, nil); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}

func TestScheduleSidebarLayoutRestoreOnExitUsesProvidedPane(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)

	tmux.EXPECT().ScheduleSidebarRestoreOnExit(ctx, "", "%9").Return(nil)

	scheduleSidebarLayoutRestoreOnExit(ctx, map[string]string{"pane": "%9"}, tmux)
}

func TestScheduleSidebarLayoutRestoreOnExitUsesTmuxPaneFallback(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockSidebarPort(t)
	t.Setenv("TMUX_PANE", "%8")

	tmux.EXPECT().ScheduleSidebarRestoreOnExit(ctx, "client-1", "%8").Return(nil)

	scheduleSidebarLayoutRestoreOnExit(ctx, map[string]string{"client": "client-1"}, tmux)
}
