package tmuxcli

import (
	"context"
	"errors"
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
			ctx := context.Background()
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
			ctx := context.Background()
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
			ctx := context.Background()
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

func TestLoadConfigFiltersProjectRoots(t *testing.T) {
	ctx := context.Background()
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
			ctx := context.Background()
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
			ctx := context.Background()
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
