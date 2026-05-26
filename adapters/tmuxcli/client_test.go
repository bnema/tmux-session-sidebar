package tmuxcli

import (
	"context"
	"errors"
	"reflect"
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

func TestListPanesParsesTmuxRows(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []ports.TmuxPaneSnapshot
	}{
		{
			name: "valid rows",
			out:  "%9\t$1\talpha\t@1\t/tmp/alpha\tbash\t0\t\t0\n%10\t$2\tbeta\t@2\t/tmp/beta\tpi\t1\t130\t1\n",
			want: []ports.TmuxPaneSnapshot{
				{PaneID: "%9", SessionID: "$1", SessionName: "alpha", WindowID: "@1", CurrentPath: "/tmp/alpha", CurrentCmd: "bash", Dead: false, Sidebar: false},
				{PaneID: "%10", SessionID: "$2", SessionName: "beta", WindowID: "@2", CurrentPath: "/tmp/beta", CurrentCmd: "pi", Dead: true, DeadStatus: "130", Sidebar: true},
			},
		},
		{name: "empty output", out: "", want: nil},
		{name: "skips malformed rows", out: "%9\t$1\talpha\n%10\t$2\tbeta\t@2\t/tmp/beta\tpi\t1\t130\t1\n", want: []ports.TmuxPaneSnapshot{{PaneID: "%10", SessionID: "$2", SessionName: "beta", WindowID: "@2", CurrentPath: "/tmp/beta", CurrentCmd: "pi", Dead: true, DeadStatus: "130", Sidebar: true}}},
		{name: "mixed valid and malformed rows", out: "bad\n%9\t$1\talpha\t@1\t/tmp/alpha\tbash\t0\t\t0\nshort\trow\n", want: []ports.TmuxPaneSnapshot{{PaneID: "%9", SessionID: "$1", SessionName: "alpha", WindowID: "@1", CurrentPath: "/tmp/alpha", CurrentCmd: "bash", Dead: false, Sidebar: false}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-F", "#{pane_id}\t#{session_id}\t#{session_name}\t#{window_id}\t#{pane_current_path}\t#{pane_current_command}\t#{pane_dead}\t#{pane_dead_status}\t#{@session-sidebar-pane}"}).Return(ports.Result{Stdout: tt.out}, nil)

			got, err := (Client{Process: process}).ListPanes(ctx)
			if err != nil {
				t.Fatalf("ListPanes error: %v", err)
			}
			if len(tt.want) == 0 && len(got) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ListPanes = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCapturePaneTextUsesTailRange(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"capture-pane", "-pJ", "-t", "%9", "-S", "-8", "-E", "-1"}).Return(ports.Result{Stdout: " line 1\n line 2\n"}, nil)

	got, err := (Client{Process: process}).CapturePaneText(ctx, "%9", 8)
	if err != nil {
		t.Fatalf("CapturePaneText error: %v", err)
	}
	if got != "line 1\n line 2" {
		t.Fatalf("CapturePaneText = %q, want trimmed pane text", got)
	}
}

func TestCapturePaneTextClampsTailLinesToMinimumOne(t *testing.T) {
	tests := []struct {
		name      string
		tailLines int
		wantStart string
	}{
		{name: "zero", tailLines: 0, wantStart: "-1"},
		{name: "negative", tailLines: -3, wantStart: "-1"},
		{name: "positive", tailLines: 3, wantStart: "-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"capture-pane", "-pJ", "-t", "%9", "-S", tt.wantStart, "-E", "-1"}).Return(ports.Result{Stdout: "line\n"}, nil)

			got, err := (Client{Process: process}).CapturePaneText(ctx, "%9", tt.tailLines)
			if err != nil {
				t.Fatalf("CapturePaneText error: %v", err)
			}
			if got != "line" {
				t.Fatalf("CapturePaneText = %q, want line", got)
			}
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

func TestWindowIDReturnsHelpfulErrorWhenTargetResolvesNoWindow(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=alpha", "#{window_id}"}).Return(ports.Result{Stdout: "\n"}, nil)

	_, err := (Client{Process: process}).WindowID(ctx, "=alpha")
	if err == nil || !strings.Contains(err.Error(), "resolve tmux window id for target \"=alpha\": empty output") {
		t.Fatalf("WindowID error = %v, want helpful empty-output error", err)
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

func TestSessionPathUsesExactSessionTarget(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=alpha", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp/project\n"}, nil)

	got, err := (Client{Process: process}).SessionPath(ctx, "alpha")
	if err != nil {
		t.Fatalf("SessionPath error: %v", err)
	}
	if got != "/tmp/project" {
		t.Fatalf("SessionPath = %q, want /tmp/project", got)
	}
}

func TestLoadConfigFiltersProjectRoots(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	expectLoadConfig(process, ctx, "b\n", "20\n", ":/a::/b:\n", "on\n")
	got, err := (Client{Process: process}).LoadConfig(ctx)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(got.ProjectRoots) != 2 || got.ProjectRoots[0] != "/a" || got.ProjectRoots[1] != "/b" {
		t.Fatalf("ProjectRoots = %#v", got.ProjectRoots)
	}
	if !got.Loaded {
		t.Fatal("Loaded = false, want true")
	}
	if !got.CloseAfterSwitch {
		t.Fatal("CloseAfterSwitch = false, want true")
	}
	if got.HeatRefreshSeconds != 5 {
		t.Fatalf("HeatRefreshSeconds = %d, want 5", got.HeatRefreshSeconds)
	}
	if got.HeatRecentHours != 1 {
		t.Fatalf("HeatRecentHours = %d, want 1", got.HeatRecentHours)
	}
	if !got.AgentAttentionEnabled {
		t.Fatal("AgentAttentionEnabled = false, want true")
	}
}

func expectLoadConfig(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string) {
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-key"}).Return(ports.Result{Stdout: key}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-width"}).Return(ports.Result{Stdout: width}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-project-roots"}).Return(ports.Result{Stdout: roots}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-close-after-switch"}).Return(ports.Result{Stdout: closeAfterSwitch}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-heat-colors"}).Return(ports.Result{Stdout: "on\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-heat-half-life-hours"}).Return(ports.Result{Stdout: "8\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-heat-stale-hours"}).Return(ports.Result{Stdout: "24\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-heat-refresh-seconds"}).Return(ports.Result{Stdout: "5\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-heat-recent-hours"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-activity-debug-log"}).Return(ports.Result{Stdout: "off\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-gvq", "@session-sidebar-agent-attention"}).Return(ports.Result{Stdout: "on\n"}, nil)
}

func TestSingletonSidebarPaneLifecycle(t *testing.T) {
	command := []string{"tmux-session-sidebar", "daemon", "serve-ui"}
	tests := []struct {
		name    string
		setup   func(context.Context, *mocks.MockProcessPort)
		call    func(context.Context, Client) (ports.PaneRef, error)
		want    ports.PaneRef
		wantErr bool
	}{
		{
			name: "find returns marked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{Stdout: "%9\t@1\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.FindSingletonSidebar(ctx)
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@1"},
		},
		{
			name: "find errors on duplicate marked panes",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{Stdout: "%9\t@1\n%10\t@2\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.FindSingletonSidebar(ctx)
			},
			wantErr: true,
		},
		{
			name: "find propagates tmux error",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{Stderr: "server exited\n"}, errors.New("tmux failed"))
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.FindSingletonSidebar(ctx)
			},
			wantErr: true,
		},
		{
			name:  "ensure refuses empty command",
			setup: func(context.Context, *mocks.MockProcessPort) {},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, nil)
			},
			wantErr: true,
		},
		{
			name: "ensure propagates find error",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{Stderr: "server exited\n"}, errors.New("tmux failed"))
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			wantErr: true,
		},
		{
			name: "ensure creates hidden session when missing",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
				process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar", "-P", "-F", "#{pane_id}\t#{window_id}", "tmux-session-sidebar", "daemon", "serve-ui"}).Return(ports.Result{Stdout: "%9\t@hidden\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@hidden"},
		},
		{
			name: "ensure cleans up created pane when marking fails",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
				process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar", "-P", "-F", "#{pane_id}\t#{window_id}", "tmux-session-sidebar", "daemon", "serve-ui"}).Return(ports.Result{Stdout: "%9\t@hidden\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{Stderr: "set failed\n"}, errors.New("set failed"))
				process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			wantErr: true,
		},
		{
			name: "ensure creates window when parking session already exists without marked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"new-window", "-d", "-t", "__tmux-session-sidebar:", "-n", "sidebar", "-P", "-F", "#{pane_id}\t#{window_id}", "tmux-session-sidebar", "daemon", "serve-ui"}).Return(ports.Result{Stdout: "%9\t@hidden\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@hidden"},
		},
		{
			name: "ensure reuses existing pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}"}).Return(ports.Result{Stdout: "%9\t@hidden\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@hidden"},
		},
		{
			name: "attach joins marked pane into client window",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-l", "20", "-s", "%9", "-t", "@1"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.AttachSingletonSidebar(ctx, "client-1", "%9", "20")
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@1"},
		},
		{
			name: "attach is no-op when pane is already in target window",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.AttachSingletonSidebar(ctx, "client-1", "%9", "20")
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@1"},
		},
		{
			name: "attach refuses unmarked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "0\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.AttachSingletonSidebar(ctx, "client-1", "%9", "20")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			tt.setup(ctx, process)
			got, err := tt.call(ctx, Client{Process: process})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("pane = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAttachSingletonSidebarAndSwitchClientRunsMoveAndSwitchInOneTmuxCommand(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	var ops []string

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{
		"join-pane", "-hbf", "-l", "20", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1",
		";", "resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
	}).Run(func(context.Context, string, []string) {
		ops = append(ops, "move-and-switch")
	}).Return(ports.Result{}, nil)

	err := (Client{Process: process}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if err != nil {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error: %v", err)
	}
	if !reflect.DeepEqual(ops, []string{"move-and-switch"}) {
		t.Fatalf("ops = %#v, want one combined move/switch command", ops)
	}
}

func TestAttachSingletonSidebarAndSwitchClientRollsBackWhenCombinedCommandFails(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	boom := errors.New("switch failed")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{
		"join-pane", "-hbf", "-l", "20", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1",
		";", "resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
	}).Return(ports.Result{Stderr: "can't find client\n"}, boom)

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil).Once()
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "source-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "source-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-l", "20", "-s", "%9", "-t", "@1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)

	err := (Client{Process: process}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error = %v, want %v", err, boom)
	}
}

func TestParkSingletonSidebar(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(context.Context, *mocks.MockProcessPort)
		wantErr bool
	}{
		{
			name: "creates hidden session then breaks marked pane into it",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
				process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar-parked"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "invalid option\n"}, errors.New("missing option"))
			},
		},
		{
			name: "restores source layout and cleans stale parking windows after parking marked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@stale\n@parked\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"kill-window", "-t", "@stale"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
			},
		},
		{
			name: "refuses unmarked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: ""}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			tt.setup(ctx, process)
			err := (Client{Process: process}).ParkSingletonSidebar(ctx, "%9")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParkSingletonSidebar error = %v, wantErr %v", err, tt.wantErr)
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
