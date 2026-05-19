package tmuxcli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestListSessionsParsesTmuxRows(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []ports.TmuxSessionSnapshot
	}{
		{name: "one row", out: "$1\talpha\t2\t1\n", want: []ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha", WindowCount: 2, AttachedCount: 1}}},
		{name: "skips malformed", out: "bad\n$2\tbeta\t1\t0\n$3\tbad\tx\t0\n", want: []ports.TmuxSessionSnapshot{{ID: "$2", Name: "beta", WindowCount: 1, AttachedCount: 0}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_windows}\t#{session_attached}"}).Return(ports.Result{Stdout: tt.out}, nil)

			got, err := (Client{Process: process}).ListSessions(ctx)
			if err != nil {
				t.Fatalf("ListSessions error: %v", err)
			}
			assertSessions(t, got, tt.want)
		})
	}
}

func TestListClientsParsesTmuxRows(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []ports.TmuxClientSnapshot
	}{
		{name: "attached client", out: "%1\t$1\t@1\t%9\talpha\n", want: []ports.TmuxClientSnapshot{{ID: "%1", CurrentSessionID: "$1", CurrentWindowID: "@1", CurrentPaneID: "%9", Attached: true}}},
		{name: "detached client", out: "%1\t$1\t@1\t%9\t\n", want: []ports.TmuxClientSnapshot{{ID: "%1", CurrentSessionID: "$1", CurrentWindowID: "@1", CurrentPaneID: "%9", Attached: false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"list-clients", "-F", "#{client_name}\t#{session_id}\t#{window_id}\t#{pane_id}\t#{client_session}"}).Return(ports.Result{Stdout: tt.out}, nil)
			got, err := (Client{Process: process}).ListClients(ctx)
			if err != nil {
				t.Fatalf("ListClients error: %v", err)
			}
			assertClients(t, got, tt.want)
		})
	}
}

func TestPaneSize(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    ports.PaneSize
		wantErr bool
	}{
		{name: "valid", out: "80\t24", want: ports.PaneSize{Width: 80, Height: 24}},
		{name: "invalid width", out: "x\t24", wantErr: true},
		{name: "invalid height", out: "80\tx", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%1", "#{pane_width}\t#{pane_height}"}).Return(ports.Result{Stdout: tt.out}, nil)
			got, err := (Client{Process: process}).PaneSize(ctx, "%1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("PaneSize error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("PaneSize = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWindowIDUsesDisplayWhenTargetIsEmpty(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)

	got, err := (Client{Process: process}).WindowID(ctx, "")
	if err != nil {
		t.Fatalf("WindowID error: %v", err)
	}
	if got != "@1" {
		t.Fatalf("WindowID = %q, want @1", got)
	}
}

func TestCurrentPanePathUsesDisplayWhenTargetIsEmpty(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp/project\n"}, nil)

	got, err := (Client{Process: process}).CurrentPanePath(ctx, "")
	if err != nil {
		t.Fatalf("CurrentPanePath error: %v", err)
	}
	if got != "/tmp/project" {
		t.Fatalf("CurrentPanePath = %q, want /tmp/project", got)
	}
}

func TestLoadConfigFiltersProjectRoots(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-key"}).Return(ports.Result{Stdout: "b\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-width"}).Return(ports.Result{Stdout: "20\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-project-roots"}).Return(ports.Result{Stdout: ":/a::/b:\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-close-after-switch"}).Return(ports.Result{Stdout: "on\n"}, nil)
	got, err := (Client{Process: process}).LoadConfig(ctx)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(got.ProjectRoots) != 2 || got.ProjectRoots[0] != "/a" || got.ProjectRoots[1] != "/b" {
		t.Fatalf("ProjectRoots = %#v", got.ProjectRoots)
	}
	if !got.CloseAfterSwitch {
		t.Fatal("CloseAfterSwitch = false, want true")
	}
}

func TestOpenSidebarPane(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
		stdout   string
		want     ports.PaneRef
		wantErr  bool
	}{
		{name: "empty output errors", stdout: "", wantErr: true},
		{name: "missing pane id errors", stdout: "\t@window\n", wantErr: true},
		{name: "targets client and parses pane", clientID: "%client", stdout: "%pane\t@window\n", want: ports.PaneRef{PaneID: "%pane", WindowID: "@window"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", mock.MatchedBy(func(args []string) bool {
				if tt.clientID == "" {
					return true
				}
				for i := 0; i < len(args)-1; i++ {
					if args[i] == "-t" && args[i+1] == tt.clientID {
						return true
					}
				}
				return false
			})).Return(ports.Result{Stdout: tt.stdout}, nil)
			got, err := (Client{Process: process}).OpenSidebarPane(ctx, tt.clientID, "20", []string{"cmd"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("OpenSidebarPane error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("OpenSidebarPane = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSaveAndRestoreWindowLayout(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	if err := client.SaveWindowLayout(ctx, "@1"); err != nil {
		t.Fatalf("SaveWindowLayout error: %v", err)
	}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
	if err := client.RestoreWindowLayout(ctx, "@1"); err != nil {
		t.Fatalf("RestoreWindowLayout error: %v", err)
	}
}

func TestSaveWindowLayout(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "current-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "current-layout"}).Return(ports.Result{}, nil)

	if err := client.SaveWindowLayout(ctx, "@1"); err != nil {
		t.Fatalf("SaveWindowLayout error: %v", err)
	}
}

func TestRestoreWindowLayoutKeepsSavedLayoutWhenSelectFails(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("select failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, boom)

	if err := client.RestoreWindowLayout(ctx, "@1"); !errors.Is(err, boom) {
		t.Fatalf("RestoreWindowLayout error = %v, want %v", err, boom)
	}
}

func TestRestoreWindowLayoutIgnoresMissingSavedLayoutOption(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "invalid option: @session-sidebar-window-layout\n", ExitCode: 1}, errors.New("tmux failed"))

	if err := client.RestoreWindowLayout(ctx, "@1"); err != nil {
		t.Fatalf("RestoreWindowLayout error = %v, want nil for missing option", err)
	}
}

func TestRestoreWindowLayoutPropagatesShowOptionsError(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @1\n", ExitCode: 1}, boom)

	if err := client.RestoreWindowLayout(ctx, "@1"); !errors.Is(err, boom) {
		t.Fatalf("RestoreWindowLayout error = %v, want %v", err, boom)
	}
}

func TestScheduleSidebarRestoreOnExit(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		if len(args) != 3 || args[0] != "run-shell" || args[1] != "-b" {
			return false
		}
		command := args[2]
		for _, want := range []string{"list-panes", "##{pane_id}", "select-layout", "set-option", "@session-sidebar-window-layout", "%9", "@1"} {
			if !strings.Contains(command, want) {
				return false
			}
		}
		return !strings.Contains(command, "seq ")
	})).Return(ports.Result{}, nil)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "", "%9"); err != nil {
		t.Fatalf("ScheduleSidebarRestoreOnExit error: %v", err)
	}
}

func TestScheduleSidebarRestoreOnExitPropagatesShowOptionsError(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "server exited unexpectedly\n", ExitCode: 1}, boom)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "", "%9"); !errors.Is(err, boom) {
		t.Fatalf("ScheduleSidebarRestoreOnExit error = %v, want %v", err, boom)
	}
}

func TestScheduleSidebarRestoreOnExitIgnoresMissingSidebarTarget(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stderr: "can't find client: client-1\n", ExitCode: 1}, boom)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "client-1", ""); err != nil {
		t.Fatalf("ScheduleSidebarRestoreOnExit error = %v, want nil for missing target", err)
	}
}

func TestScheduleSidebarRestoreOnExitPropagatesSidebarLookupError(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stderr: "server exited unexpectedly\n", ExitCode: 1}, boom)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "client-1", ""); !errors.Is(err, boom) {
		t.Fatalf("ScheduleSidebarRestoreOnExit error = %v, want %v", err, boom)
	}
}

func TestScheduleSidebarRestoreOnExitIgnoresMissingPaneTarget(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("tmux failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stderr: "can't find pane: %9\n", ExitCode: 1}, boom)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "", "%9"); err != nil {
		t.Fatalf("ScheduleSidebarRestoreOnExit error = %v, want nil for missing pane", err)
	}
}

func TestOpenSidebarRestoresLayoutWhenSplitOutputIsMalformed(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-key"}).Return(ports.Result{Stdout: "M-b\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-width"}).Return(ports.Result{Stdout: "20\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-project-roots"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-close-after-switch"}).Return(ports.Result{Stdout: "off\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"split-window", "-P", "-F", "#{pane_id}\t#{window_id}", "-t", "@1", "-hbf", "-l", "20", "-c", "/tmp", "ui", "run"}).Return(ports.Result{Stdout: "\t@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	if _, err := client.OpenSidebar(ctx, "client-1", []string{"ui", "run"}); err == nil {
		t.Fatal("OpenSidebar error = nil, want malformed split output error")
	}
}

func TestOpenSidebarResizesMarkedSidebarPaneToConfiguredWidth(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-key"}).Return(ports.Result{Stdout: "M-b\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-width"}).Return(ports.Result{Stdout: "20\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-project-roots"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-close-after-switch"}).Return(ports.Result{Stdout: "off\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"split-window", "-P", "-F", "#{pane_id}\t#{window_id}", "-t", "@1", "-hbf", "-l", "20", "-c", "/tmp", "ui", "run"}).Return(ports.Result{Stdout: "%9\t@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)

	got, err := client.OpenSidebar(ctx, "client-1", []string{"ui", "run"})
	if err != nil {
		t.Fatalf("OpenSidebar error: %v", err)
	}
	if got != (ports.PaneRef{PaneID: "%9", WindowID: "@1"}) {
		t.Fatalf("OpenSidebar = %#v, want pane ref", got)
	}
}

func TestOpenSidebarCleansUpWhenPaneMarkFails(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("mark failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-key"}).Return(ports.Result{Stdout: "M-b\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-width"}).Return(ports.Result{Stdout: "20\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-project-roots"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-close-after-switch"}).Return(ports.Result{Stdout: "off\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"split-window", "-P", "-F", "#{pane_id}\t#{window_id}", "-t", "@1", "-hbf", "-l", "20", "-c", "/tmp", "ui", "run"}).Return(ports.Result{Stdout: "%9\t@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, boom)
	process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	if _, err := client.OpenSidebar(ctx, "client-1", []string{"ui", "run"}); !errors.Is(err, boom) {
		t.Fatalf("OpenSidebar error = %v, want %v", err, boom)
	}
}

func TestCloseSidebarPaneReturnsWindowLookupError(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("window lookup failed")
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{}, boom)

	if err := client.CloseSidebarPane(ctx, "%9"); !errors.Is(err, boom) {
		t.Fatalf("CloseSidebarPane error = %v, want %v", err, boom)
	}
}

func TestCloseSidebarPaneSchedulesRestoreBeforeKillingPane(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}
	restoreScheduled := false

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 3 && args[0] == "run-shell" && args[1] == "-b" && strings.Contains(args[2], "%9")
	})).Run(func(context.Context, string, []string) {
		restoreScheduled = true
	}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Run(func(context.Context, string, []string) {
		if !restoreScheduled {
			t.Fatal("kill-pane ran before restore watcher was scheduled")
		}
	}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	if err := client.CloseSidebarPane(ctx, "%9"); err != nil {
		t.Fatalf("CloseSidebarPane error: %v", err)
	}
}

func TestCloseSidebarPaneIgnoresInvalidSavedLayoutAfterPaneCountChanges(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}
	restoreErr := errors.New("invalid saved layout")

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 3 && args[0] == "run-shell" && args[1] == "-b" && strings.Contains(args[2], "%9")
	})).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, restoreErr)

	if err := client.CloseSidebarPane(ctx, "%9"); err != nil {
		t.Fatalf("CloseSidebarPane error = %v, want nil", err)
	}
}

func TestExecErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		call func(context.Context, Client) error
	}{
		{name: "list sessions", call: func(ctx context.Context, c Client) error { _, err := c.ListSessions(ctx); return err }},
		{name: "list clients", call: func(ctx context.Context, c Client) error { _, err := c.ListClients(ctx); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", mock.Anything).Return(ports.Result{}, boom)
			if err := tt.call(ctx, Client{Process: process}); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want %v", err, boom)
			}
		})
	}
}

func assertSessions(t *testing.T, got []ports.TmuxSessionSnapshot, want []ports.TmuxSessionSnapshot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertClients(t *testing.T, got []ports.TmuxClientSnapshot, want []ports.TmuxClientSnapshot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("client[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
