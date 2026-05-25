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

func TestOpenSidebarDelegatesToPortWithClientArg(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockTmuxSidebarPort(t)

	tmux.EXPECT().OpenSidebar(ctx, "client-1", mock.MatchedBy(matchesUIRunCommand("client-1"))).Return(ports.PaneRef{PaneID: "%10", WindowID: "@1"}, nil)

	if err := openSidebar(ctx, map[string]string{"client": "client-1"}, tmux); err != nil {
		t.Fatalf("openSidebar returned error: %v", err)
	}
}

func TestCloseSidebarDelegatesToPort(t *testing.T) {
	ctx := t.Context()
	tmux := mocks.NewMockTmuxSidebarPort(t)

	tmux.EXPECT().CloseSidebar(ctx, "client-1").Return(nil)

	if err := closeSidebar(ctx, map[string]string{"client": "client-1"}, tmux); err != nil {
		t.Fatalf("closeSidebar returned error: %v", err)
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
