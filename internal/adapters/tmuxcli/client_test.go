package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func allowMissingWindowLayoutOption(process *mocks.MockProcessPort, ctx context.Context, option string) {
	// Matches: show-options -w -v -t <target> <option>
	process.On("Exec", ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 6 && args[0] == "show-options" && args[1] == "-w" && args[2] == "-v" && args[3] == "-t" && args[4] != "" && args[5] == option
	})).Return(ports.Result{Stderr: "invalid option\n"}, errors.New("missing option")).Maybe()
}

func allowSidebarOpenBaselineCaptureMaybe(process *mocks.MockProcessPort, ctx context.Context, windowID string, paneID string, width string) {
	process.On("Exec", ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 6 && args[0] == "show-options" && args[1] == "-w" && args[2] == "-v" && args[3] == "-t" && (windowID == "" || args[4] == windowID) && args[5] == optionSidebarResizeSyncActive
	})).Return(ports.Result{Stderr: "invalid option\n"}, errors.New("missing option")).Maybe()
	allowSidebarCloseRebalanceCaptureMaybe(process, ctx, windowID, paneID, width)
	process.On("Exec", ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 5 && args[0] == "set-option" && args[1] == "-wu" && args[2] == "-t" && (windowID == "" || args[3] == windowID) && args[4] == optionSidebarOpenWorkBaseline
	})).Return(ports.Result{}, nil).Maybe()
}

func allowSidebarCloseRebalanceCaptureMaybe(process *mocks.MockProcessPort, ctx context.Context, windowID string, paneID string, width string) {
	width = strings.TrimSpace(width)
	if width == "" {
		width = "30"
	}
	parsedWidth, err := strconv.Atoi(width)
	if err != nil || parsedWidth <= 0 {
		parsedWidth = 30
	}
	process.On("Exec", ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 5 && args[0] == "display-message" && args[1] == "-p" && args[2] == "-t" && (windowID == "" || args[3] == windowID) && args[4] == "#{window_width}\t#{window_height}"
	})).Return(ports.Result{Stdout: "100\t30\n"}, nil).Maybe()
	process.On("Exec", ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		return len(args) == 5 && args[0] == "list-panes" && args[1] == "-t" && (windowID == "" || args[2] == windowID) && args[3] == "-F" && args[4] == formatSidebarRebalancePane
	})).Return(ports.Result{Stdout: fmt.Sprintf("%s\t0\t0\t%d\t30\t1\n%%1\t%d\t0\t%d\t30\t0\n", paneID, parsedWidth, parsedWidth+1, 99-parsedWidth)}, nil).Maybe()
}

func TestListSessionsParsesTmuxRows(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []ports.SessionSnapshot
	}{
		{name: "one row", out: "$1\talpha\t2\t1\n", want: []ports.SessionSnapshot{{ID: "$1", Name: "alpha", WindowCount: 2, AttachedCount: 1}}},
		{name: "skips malformed", out: "bad\n$2\tbeta\t1\t0\n$3\tbad\tx\t0\n", want: []ports.SessionSnapshot{{ID: "$2", Name: "beta", WindowCount: 1, AttachedCount: 0}}},
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
		want []ports.ClientSnapshot
	}{
		{name: "attached client", out: "%1\t$1\t@1\t%9\talpha\n", want: []ports.ClientSnapshot{{ID: "%1", CurrentSessionID: "$1", CurrentWindowID: "@1", CurrentPaneID: "%9", Attached: true}}},
		{name: "detached client", out: "%1\t$1\t@1\t%9\t\n", want: []ports.ClientSnapshot{{ID: "%1", CurrentSessionID: "$1", CurrentWindowID: "@1", CurrentPaneID: "%9", Attached: false}}},
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
		want []ports.PaneSnapshot
	}{
		{
			name: "valid rows",
			out:  "%9\t$1\talpha\t@1\t/tmp/alpha\tbash\t0\t\t0\n%10\t$2\tbeta\t@2\t/tmp/beta\tpi\t1\t130\t1\n",
			want: []ports.PaneSnapshot{
				{PaneID: "%9", SessionID: "$1", SessionName: "alpha", WindowID: "@1", CurrentPath: "/tmp/alpha", CurrentCmd: "bash", Dead: false, Sidebar: false},
				{PaneID: "%10", SessionID: "$2", SessionName: "beta", WindowID: "@2", CurrentPath: "/tmp/beta", CurrentCmd: "pi", Dead: true, DeadStatus: "130", Sidebar: true},
			},
		},
		{name: "empty output", out: "", want: nil},
		{name: "skips malformed rows", out: "%9\t$1\talpha\n%10\t$2\tbeta\t@2\t/tmp/beta\tpi\t1\t130\t1\n", want: []ports.PaneSnapshot{{PaneID: "%10", SessionID: "$2", SessionName: "beta", WindowID: "@2", CurrentPath: "/tmp/beta", CurrentCmd: "pi", Dead: true, DeadStatus: "130", Sidebar: true}}},
		{name: "mixed valid and malformed rows", out: "bad\n%9\t$1\talpha\t@1\t/tmp/alpha\tbash\t0\t\t0\nshort\trow\n", want: []ports.PaneSnapshot{{PaneID: "%9", SessionID: "$1", SessionName: "alpha", WindowID: "@1", CurrentPath: "/tmp/alpha", CurrentCmd: "bash", Dead: false, Sidebar: false}}},
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

func TestWindowIDTreatsEmptyOutputForConcreteWindowTargetAsTargetGone(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@351", "#{window_id}"}).Return(ports.Result{Stdout: "\n"}, nil)

	_, err := (Client{Process: process}).WindowID(ctx, "@351")
	if !errors.Is(err, ports.ErrMultiplexerTargetGone) {
		t.Fatalf("WindowID error = %v, want ErrMultiplexerTargetGone", err)
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

func TestSessionPathUsesExactSessionWindowTarget(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=alpha:", "#{pane_current_path}"}).Return(ports.Result{Stdout: "/tmp/project\n"}, nil)

	got, err := (Client{Process: process}).SessionPath(ctx, "alpha")
	if err != nil {
		t.Fatalf("SessionPath error: %v", err)
	}
	if got != "/tmp/project" {
		t.Fatalf("SessionPath = %q, want /tmp/project", got)
	}
}

func TestSessionPathsUsesOneListPanesCallForActivePanePaths(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-F", "#{session_name}\t#{window_active}\t#{pane_active}\t#{pane_current_path}"}).Return(ports.Result{Stdout: "alpha\t1\t0\t/tmp/not-active-pane\nalpha\t1\t1\t/tmp/alpha\nbeta\t0\t1\t/tmp/not-active-window\nbeta\t1\t1\t/tmp/beta\ngamma\t1\t1\t/tmp/gamma\nmalformed\n"}, nil)

	got, err := (Client{Process: process}).SessionPaths(ctx, []string{"alpha", "beta", "missing"})
	if err != nil {
		t.Fatalf("SessionPaths error: %v", err)
	}
	want := map[string]string{"alpha": "/tmp/alpha", "beta": "/tmp/beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionPaths = %#v, want %#v", got, want)
	}
}

func TestSessionPathsPreservesExactSessionNameIdentity(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-F", "#{session_name}\t#{window_active}\t#{pane_active}\t#{pane_current_path}"}).Return(ports.Result{Stdout: "other\t1\t1\t/tmp/other\n alpha \t1\t1\t/tmp/spaced\nalpha\t1\t1\t/tmp/plain\n"}, nil)

	got, err := (Client{Process: process}).SessionPaths(ctx, []string{" alpha "})
	if err != nil {
		t.Fatalf("SessionPaths error: %v", err)
	}
	want := map[string]string{" alpha ": "/tmp/spaced"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionPaths = %#v, want %#v", got, want)
	}
}

func TestSwitchClientSessionUsesExactSessionTarget(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	process.EXPECT().Exec(ctx, "tmux", []string{"switch-client", "-c", "client-1", "-t", "=alpha:"}).Return(ports.Result{}, nil)

	if err := (Client{Process: process}).SwitchClientSession(ctx, "client-1", "alpha"); err != nil {
		t.Fatalf("SwitchClientSession error: %v", err)
	}
}

func TestLoadConfigFiltersProjectRoots(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	expectLoadConfig(process, ctx, "b\n", "30\n", ":/a::/b:\n", "on\n", "on\n")
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
	if got.HeatRefreshSeconds != 60 {
		t.Fatalf("HeatRefreshSeconds = %d, want 60", got.HeatRefreshSeconds)
	}
	if got.HeatRecentInterval != time.Hour {
		t.Fatalf("HeatRecentInterval = %v, want 1h", got.HeatRecentInterval)
	}
	if got.HeatMaxHighlighted != 0 {
		t.Fatalf("HeatMaxHighlighted = %d, want 0", got.HeatMaxHighlighted)
	}
	if !got.AgentAttentionEnabled {
		t.Fatal("AgentAttentionEnabled = false, want true")
	}
	if got.AutoSortRecentInterval != 24*time.Hour {
		t.Fatalf("AutoSortRecentInterval = %v, want 24h", got.AutoSortRecentInterval)
	}
	if !got.MetadataSublineEnabled {
		t.Fatal("MetadataSublineEnabled = false, want true")
	}
}

func TestLoadConfigParsesMetadataSublineBool(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want bool
	}{
		"default empty is on": {raw: "\n", want: true},
		"on":                  {raw: "on\n", want: true},
		"off":                 {raw: "off\n", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			expectLoadConfigWithMetadata(process, ctx, "b\n", "30\n", "\n", "off\n", "off\n", tt.raw)

			got, err := (Client{Process: process}).LoadConfig(ctx)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}
			if got.MetadataSublineEnabled != tt.want {
				t.Fatalf("MetadataSublineEnabled = %v, want %v", got.MetadataSublineEnabled, tt.want)
			}
		})
	}
}

func TestLoadConfigParsesMetadataInactiveBool(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want bool
	}{
		"default empty is on": {raw: "\n", want: true},
		"on":                  {raw: "on\n", want: true},
		"off":                 {raw: "off\n", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			expectLoadConfigWithMetadataInactive(process, ctx, "b\n", "30\n", "\n", "off\n", "off\n", "on\n", tt.raw)

			got, err := (Client{Process: process}).LoadConfig(ctx)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}
			if got.MetadataInactiveEnabled != tt.want {
				t.Fatalf("MetadataInactiveEnabled = %v, want %v", got.MetadataInactiveEnabled, tt.want)
			}
		})
	}
}

func TestLoadConfigParsesColorSchemeMode(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want config.ColorSchemeMode
	}{
		"default empty is system": {raw: "\n", want: config.ColorSchemeModeSystem},
		"system":                  {raw: "system\n", want: config.ColorSchemeModeSystem},
		"light":                   {raw: "light\n", want: config.ColorSchemeModeLight},
		"dark":                    {raw: "dark\n", want: config.ColorSchemeModeDark},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			expectLoadConfigWithColorScheme(process, ctx, "b\n", "30\n", "\n", "off\n", "off\n", "on\n", "off\n", "pulse\n", tt.raw)

			got, err := (Client{Process: process}).LoadConfig(ctx)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}
			if got.ColorSchemeMode != tt.want {
				t.Fatalf("ColorSchemeMode = %q, want %q", got.ColorSchemeMode, tt.want)
			}
		})
	}
}

func TestLoadConfigParsesAgentAttentionAnimation(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want config.AgentAttentionAnimation
	}{
		"default empty is pulse": {raw: "\n", want: config.AgentAttentionAnimationPulse},
		"rainbow":                {raw: "rainbow\n", want: config.AgentAttentionAnimationRainbow},
		"blink":                  {raw: "blink\n", want: config.AgentAttentionAnimationBlink},
		"off":                    {raw: "off\n", want: config.AgentAttentionAnimationOff},
		"unknown disables":       {raw: "sparkle\n", want: config.AgentAttentionAnimationOff},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			expectLoadConfigWithAttentionAnimation(process, ctx, "b\n", "30\n", "\n", "off\n", "off\n", "on\n", "off\n", tt.raw)

			got, err := (Client{Process: process}).LoadConfig(ctx)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}
			if got.AgentAttentionAnimation != tt.want {
				t.Fatalf("AgentAttentionAnimation = %q, want %q", got.AgentAttentionAnimation, tt.want)
			}
		})
	}
}

func TestLoadConfigParsesHeatRecentInterval(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want time.Duration
	}{
		"empty default":   {raw: "", want: time.Hour},
		"legacy one":      {raw: "1", want: time.Hour},
		"minutes":         {raw: "10m", want: 10 * time.Minute},
		"hours":           {raw: "2h", want: 2 * time.Hour},
		"days":            {raw: "3d", want: 72 * time.Hour},
		"invalid default": {raw: "bogus", want: time.Hour},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := parseHeatRecentInterval(tt.raw); got != tt.want {
				t.Fatalf("parseHeatRecentInterval(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLoadConfigParsesAutoSortRecentInterval(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want time.Duration
	}{
		"off":     {raw: "off\n", want: 0},
		"empty":   {raw: "\n", want: 0},
		"on":      {raw: "on\n", want: 24 * time.Hour},
		"minutes": {raw: "10m\n", want: 10 * time.Minute},
		"hours":   {raw: "2h\n", want: 2 * time.Hour},
		"days":    {raw: "3d\n", want: 72 * time.Hour},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			process := mocks.NewMockProcessPort(t)
			expectLoadConfig(process, ctx, "b\n", "30\n", "\n", "off\n", tt.raw)

			got, err := (Client{Process: process}).LoadConfig(ctx)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}
			if got.AutoSortRecentInterval != tt.want {
				t.Fatalf("AutoSortRecentInterval = %v, want %v", got.AutoSortRecentInterval, tt.want)
			}
		})
	}
}

func TestLoadOptionsMapPreservesEmptyValues(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	stdout := strings.Join([]string{
		"",
		"30",
		"",
		"off",
		"on",
		"8",
		"24",
		"60",
		"1h",
		"0",
		"off",
		"on",
		"pulse",
		"off",
		"auto",
		"3",
		"system",
		"",
		"off",
	}, configQuerySeparator) + "\n"
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", tmuxConfigQueryFormat()}).Return(ports.Result{Stdout: stdout}, nil)

	opts, err := (Client{Process: process}).loadOptionsMap(ctx)
	if err != nil {
		t.Fatalf("loadOptionsMap error: %v", err)
	}
	if opts["@session-sidebar-key"] != "" {
		t.Fatalf("key = %q, want empty value", opts["@session-sidebar-key"])
	}
	if opts["@session-sidebar-width"] != "30" {
		t.Fatalf("width = %q, want 30", opts["@session-sidebar-width"])
	}
	if opts["@session-sidebar-project-roots"] != "" {
		t.Fatalf("project-roots = %q, want empty value", opts["@session-sidebar-project-roots"])
	}
	if opts["@session-sidebar-close-after-switch"] != "off" {
		t.Fatalf("close-after-switch = %q, want off", opts["@session-sidebar-close-after-switch"])
	}
}

func TestLoadConfigParsesProjectRootsWithSpaces(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	// The roots value contains internal spaces.
	expectLoadConfig(process, ctx, "b\n", "30\n", "/home/user/my project:/work\n", "off\n", "off\n")
	got, err := (Client{Process: process}).LoadConfig(ctx)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(got.ProjectRoots) != 2 {
		t.Fatalf("ProjectRoots len = %d, want 2: %#v", len(got.ProjectRoots), got.ProjectRoots)
	}
	if got.ProjectRoots[0] != "/home/user/my project" {
		t.Fatalf("ProjectRoots[0] = %q, want /home/user/my project", got.ProjectRoots[0])
	}
	if got.ProjectRoots[1] != "/work" {
		t.Fatalf("ProjectRoots[1] = %q, want /work", got.ProjectRoots[1])
	}
}

func TestLoadConfigPreservesBackslashesAndExplicitEmptyValues(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	stdout := strings.Join([]string{
		"b",
		"30",
		"/tmp/a\\b:/home/user/my project",
		"off",
		"on",
		"8",
		"24",
		"60",
		"1h",
		"0",
		"off",
		"on",
		"pulse",
		"off",
		"auto",
		"3",
		"system",
		"",
		"off",
	}, configQuerySeparator) + "\n"
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", tmuxConfigQueryFormat()}).Return(ports.Result{Stdout: stdout}, nil)

	got, err := (Client{Process: process}).LoadConfig(ctx)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(got.ProjectRoots) != 2 {
		t.Fatalf("ProjectRoots len = %d, want 2: %#v", len(got.ProjectRoots), got.ProjectRoots)
	}
	if got.ProjectRoots[0] != "/tmp/a\\b" {
		t.Fatalf("ProjectRoots[0] = %q, want /tmp/a\\b", got.ProjectRoots[0])
	}
	if got.ProjectRoots[1] != "/home/user/my project" {
		t.Fatalf("ProjectRoots[1] = %q, want /home/user/my project", got.ProjectRoots[1])
	}
	if !got.MetadataSublineEnabled {
		t.Fatal("MetadataSublineEnabled = false, want default-on behavior for explicit empty string")
	}
}

func expectLoadConfig(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string, autoSortRecent string) {
	expectLoadConfigWithMetadata(process, ctx, key, width, roots, closeAfterSwitch, autoSortRecent, "on\n")
}

func expectLoadConfigWithMetadata(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string, autoSortRecent string, metadataSubline string) {
	expectLoadConfigWithMetadataInactive(process, ctx, key, width, roots, closeAfterSwitch, autoSortRecent, metadataSubline, "off\n")
}

func expectLoadConfigWithMetadataInactive(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string, autoSortRecent string, metadataSubline string, metadataInactive string) {
	expectLoadConfigWithAttentionAnimation(process, ctx, key, width, roots, closeAfterSwitch, autoSortRecent, metadataSubline, metadataInactive, "pulse\n")
}

func expectLoadConfigWithAttentionAnimation(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string, autoSortRecent string, metadataSubline string, metadataInactive string, attentionAnimation string) {
	expectLoadConfigWithColorScheme(process, ctx, key, width, roots, closeAfterSwitch, autoSortRecent, metadataSubline, metadataInactive, attentionAnimation, "system\n")
}

func expectLoadConfigWithColorScheme(process *mocks.MockProcessPort, ctx context.Context, key string, width string, roots string, closeAfterSwitch string, autoSortRecent string, metadataSubline string, metadataInactive string, attentionAnimation string, colorScheme string) {
	stdout := strings.Join([]string{
		strings.TrimSpace(key),
		strings.TrimSpace(width),
		strings.TrimSpace(roots),
		strings.TrimSpace(closeAfterSwitch),
		"on",
		"8",
		"24",
		"60",
		"1h",
		"0",
		"off",
		"on",
		strings.TrimSpace(attentionAnimation),
		strings.TrimSpace(autoSortRecent),
		"auto",
		"3",
		strings.TrimSpace(colorScheme),
		strings.TrimSpace(metadataSubline),
		strings.TrimSpace(metadataInactive),
	}, configQuerySeparator) + "\n"
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", tmuxConfigQueryFormat()}).Return(ports.Result{Stdout: stdout}, nil)
}

func TestFindSidebarPaneIgnoresDeadMarkedPane(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", "#{pane_id}\t#{@session-sidebar-pane}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t1\t1\n%10\t0\t0\n"}, nil)

	got, err := client.FindSidebarPane(ctx, "client-1")
	if err != nil {
		t.Fatalf("FindSidebarPane error: %v", err)
	}
	if got.PaneID != "" || got.WindowID != "@1" {
		t.Fatalf("FindSidebarPane = %#v, want no live sidebar pane in @1", got)
	}
}

func TestFindSidebarPaneReturnsMarkedPaneRegardlessOfOwner(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", "#{pane_id}\t#{@session-sidebar-pane}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t1\t0\n"}, nil)

	got, err := client.FindSidebarPane(ctx, "client-1")
	if err != nil {
		t.Fatalf("FindSidebarPane error: %v", err)
	}
	if got != (ports.PaneRef{PaneID: "%9", WindowID: "@1"}) {
		t.Fatalf("FindSidebarPane = %#v, want owner-agnostic marked pane", got)
	}
}

func TestOwnerScopedSidebarPaneLifecycle(t *testing.T) {
	command := []string{"tmux-session-sidebar", "daemon", "serve-ui"}
	t.Run("find filters by owner and cleans dead panes", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{&&:#{==:#{@session-sidebar-pane},1},#{==:#{@session-sidebar-owner-client},client-A}}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@1\t1\n%10\t@2\t0\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).FindSidebarPaneForClient(ctx, "client-A")
		if err != nil {
			t.Fatalf("FindSidebarPaneForClient error: %v", err)
		}
		if got != (ports.PaneRef{PaneID: "%10", WindowID: "@2"}) {
			t.Fatalf("FindSidebarPaneForClient = %#v, want live owner-scoped pane", got)
		}
	})

	t.Run("ensure creates and marks sidebar owner when no owner pane exists", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{&&:#{==:#{@session-sidebar-pane},1},#{==:#{@session-sidebar-owner-client},client-A}}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
		process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar", "-P", "-F", "#{pane_id}\t#{window_id}", "tmux-session-sidebar", "daemon", "serve-ui"}).Return(ports.Result{Stdout: "%9\t@hidden\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1", ";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-A"}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).EnsureSidebarForClient(ctx, "client-A", command)
		if err != nil {
			t.Fatalf("EnsureSidebarForClient error: %v", err)
		}
		if got != (ports.PaneRef{PaneID: "%9", WindowID: "@hidden"}) {
			t.Fatalf("EnsureSidebarForClient = %#v, want created pane", got)
		}
	})

	t.Run("ensure reuses only matching owner pane", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{&&:#{==:#{@session-sidebar-pane},1},#{==:#{@session-sidebar-owner-client},client-A}}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@hidden\t0\n"}, nil)

		got, err := (Client{Process: process}).EnsureSidebarForClient(ctx, "client-A", command)
		if err != nil {
			t.Fatalf("EnsureSidebarForClient error: %v", err)
		}
		if got != (ports.PaneRef{PaneID: "%9", WindowID: "@hidden"}) {
			t.Fatalf("EnsureSidebarForClient = %#v, want matching owner pane", got)
		}
	})

	t.Run("attach marks owner on pane", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1", ";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-A"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "target-window", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).AttachSidebarForClient(ctx, "client-A", "target-window", "%9", "20")
		if err != nil {
			t.Fatalf("AttachSidebarForClient error: %v", err)
		}
		if got != (ports.PaneRef{PaneID: "%9", WindowID: "@1"}) {
			t.Fatalf("AttachSidebarForClient = %#v, want attached pane", got)
		}
	})

	t.Run("park keeps owner marker before moving pane", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		allowSidebarCloseRebalanceCaptureMaybe(process, ctx, "@1", "%9", "")
		process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1", ";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-A"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)

		if err := (Client{Process: process}).ParkSidebarForClient(ctx, "client-A", "%9"); err != nil {
			t.Fatalf("ParkSidebarForClient error: %v", err)
		}
	})
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
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@1\t0\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.FindSingletonSidebar(ctx)
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@1"},
		},
		{
			name: "find errors on duplicate marked panes",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@1\t0\n%10\t@2\t0\n"}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.FindSingletonSidebar(ctx)
			},
			wantErr: true,
		},
		{
			name: "find propagates tmux error",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stderr: "server exited\n"}, errors.New("tmux failed"))
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
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stderr: "server exited\n"}, errors.New("tmux failed"))
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			wantErr: true,
		},
		{
			name: "ensure kills dead marked pane before creating replacement",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@1\t1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"kill-pane", "-t", "%9"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
				process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar", "-P", "-F", "#{pane_id}\t#{window_id}", "tmux-session-sidebar", "daemon", "serve-ui"}).Return(ports.Result{Stdout: "%10\t@hidden\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%10", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.EnsureSingletonSidebar(ctx, command)
			},
			want: ports.PaneRef{PaneID: "%10", WindowID: "@hidden"},
		},
		{
			name: "ensure creates hidden session when missing",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{}, nil)
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
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{}, nil)
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
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{}, nil)
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
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@hidden\t0\n"}, nil)
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
				process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@1"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))
			},
			call: func(ctx context.Context, client Client) (ports.PaneRef, error) {
				return client.AttachSingletonSidebar(ctx, "client-1", "%9", "20")
			},
			want: ports.PaneRef{PaneID: "%9", WindowID: "@1"},
		}, // Visible-layout replay expectations are intentionally absent: attach now relies on the live tmux layout plus explicit sidebar-width resize.
		{
			name: "attach is no-op when pane is already in target window",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
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
			allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")
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

func TestAttachSingletonSidebarWithoutFocusSelectsPaneRightOfSidebar(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9", "-R"}).Return(ports.Result{}, nil)

	got, err := (Client{Process: process}).AttachSingletonSidebarWithoutFocus(ctx, "client-1", "%9", "20")
	if err != nil {
		t.Fatalf("AttachSingletonSidebarWithoutFocus error: %v", err)
	}
	if got != (ports.PaneRef{PaneID: "%9", WindowID: "@1"}) {
		t.Fatalf("pane = %#v, want target window ref", got)
	}
}

func TestAttachSingletonSidebarClearsSavedTargetLayoutWhenJoinFails(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarCloseRebalanceCaptureMaybe(process, ctx, "@1", "%9", "20")
	boom := errors.New("join failed")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-layout-before-sidebar"}).Return(ports.Result{}, nil)
	allowMissingWindowLayoutOption(process, ctx, optionSidebarWindowLayout)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{Stderr: "can't join pane\n"}, boom)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@2", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	_, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebar error = %v, want %v", err, boom)
	}
}

func TestAttachSingletonSidebarIgnoresMissingSourceWindowAfterJoin(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))

	got, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20")
	if err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
	if got != (ports.PaneRef{PaneID: "%9", WindowID: "@2"}) {
		t.Fatalf("pane = %#v, want target window ref", got)
	}
}

func TestAttachSingletonSidebarLeavesSourceWindowOnNativeTmuxLayoutAfterMove(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	got, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20")
	if err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
	if got != (ports.PaneRef{PaneID: "%9", WindowID: "@2"}) {
		t.Fatalf("pane = %#v, want target window ref", got)
	}
	process.AssertNotCalled(t, "Exec", ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarWindowLayout})
	process.AssertNotCalled(t, "Exec", ctx, "tmux", []string{"select-layout", "-t", "@1", "source-layout-before-sidebar"})
}

func TestAttachSingletonSidebarOverwritesStaleTargetHiddenLayoutBeforeJoin(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "fresh-target-hidden-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "fresh-target-hidden-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))

	if _, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
}

func TestAttachSingletonSidebarResizesWidthAfterJoin(t *testing.T) {
	// After join-pane, the sidebar width is set explicitly and the work-area
	// layout is left to tmux's native redistribution; there is no visible-layout replay.
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-hidden-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-hidden-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))

	if _, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
	// The obsolete visible-layout option is gone, so there is no visible-layout assertion here.
}

func TestAttachSingletonSidebarCapturesSidebarOpenBaselineAfterJoin(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarCloseRebalanceCaptureMaybe(process, ctx, "@hidden", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-hidden-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-hidden-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@2", optionSidebarResizeSyncActive}).Return(ports.Result{Stdout: "0\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_width}\t#{window_height}"}).Return(ports.Result{Stdout: "100\t30\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@2", "-F", formatSidebarRebalancePane}).Return(ports.Result{Stdout: "%9\t0\t0\t20\t30\t1\n%11\t21\t0\t39\t30\t0\n%12\t61\t0\t39\t15\t0\n%13\t61\t16\t39\t14\t0\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", optionSidebarOpenWorkBaseline, `{"representativePaneIDs":["%11","%12"],"workWidths":[39,39]}`}).Return(ports.Result{}, nil)

	if _, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
}

func TestAttachSingletonSidebarUsesDirectWidthAfterJoin_NoVisibleLayoutSwap(t *testing.T) {
	// There is no visible-layout swap path here. The mock asserts that direct
	// width resize is used unconditionally after join-pane.
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@hidden\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-hidden-layout\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "target-hidden-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-pane", "-t", "%9"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@hidden", "@session-sidebar-window-layout"}).Return(ports.Result{Stderr: "no such window: @hidden\n"}, errors.New("no such window"))

	if _, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
}

func TestAttachSingletonSidebarRollsBackToSourceWindowAfterPostJoinFailures(t *testing.T) {
	tests := []struct {
		name          string
		failCommand   []string
		beforeFailure func(context.Context, *mocks.MockProcessPort)
	}{
		{
			name:        "mark sidebar pane",
			failCommand: []string{"set-option", "-p", "-t", "%9", optionSidebarPane, "1"},
		},
		{
			name: "resize sidebar width",
			beforeFailure: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", optionSidebarPane, "1"}).Return(ports.Result{}, nil)
			},
			failCommand: []string{"resize-pane", "-t", "%9", "-x", "20"},
		},
		{
			name: "select attached sidebar pane",
			beforeFailure: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-p", "-t", "%9", optionSidebarPane, "1"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"resize-pane", "-t", "%9", "-x", "20"}).Return(ports.Result{}, nil)
			},
			failCommand: []string{"select-pane", "-t", "%9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			boom := errors.New(tt.name + " failed")
			process := mocks.NewMockProcessPort(t)
			allowSidebarCloseRebalanceCaptureMaybe(process, ctx, "@1", "%9", "20")

			process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", optionSidebarPane}).Return(ports.Result{Stdout: "1\n"}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "target-layout-before-sidebar\n"}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", optionSidebarWindowLayout, "target-layout-before-sidebar"}).Return(ports.Result{}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2"}).Return(ports.Result{}, nil)
			if tt.beforeFailure != nil {
				tt.beforeFailure(ctx, process)
			}
			process.EXPECT().Exec(ctx, "tmux", tt.failCommand).Return(ports.Result{Stderr: tt.name + " failed\n"}, boom)
			process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout}).Return(ports.Result{}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@1"}).Return(ports.Result{}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@2", optionSidebarWindowLayout}).Return(ports.Result{Stdout: "target-layout-before-sidebar\n"}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@2", "target-layout-before-sidebar"}).Return(ports.Result{}, nil)
			process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@2", optionSidebarWindowLayout}).Return(ports.Result{}, nil)

			_, err := (Client{Process: process}).AttachSingletonSidebar(ctx, "client-1", "%9", "20")
			if !errors.Is(err, boom) {
				t.Fatalf("AttachSingletonSidebar error = %v, want %v", err, boom)
			}
		})
	}
}

func TestAttachSidebarForClientAndSwitchClientRejectsBlankClient(t *testing.T) {
	err := (Client{Process: mocks.NewMockProcessPort(t)}).AttachSidebarForClientAndSwitchClient(t.Context(), " \t", "beta", "%9", "20")
	if err == nil || !strings.Contains(err.Error(), "missing sidebar owner client") {
		t.Fatalf("AttachSidebarForClientAndSwitchClient error = %v, want missing owner client", err)
	}
}

func TestAttachSingletonSidebarAndSwitchClientRunsMoveAndSwitchInOneTmuxCommand(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	allowSidebarOpenBaselineCaptureMaybe(process, ctx, "", "%9", "20")
	var ops []string

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@2", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@2", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{
		"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-1",
		";", "resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "select-pane", "-t", "%9", "-R",
	}).Run(func(context.Context, string, []string) {
		ops = append(ops, "move-and-switch")
	}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)

	err := (Client{Process: process}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if err != nil {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error: %v", err)
	}
	if !reflect.DeepEqual(ops, []string{"move-and-switch"}) {
		t.Fatalf("ops = %#v, want one combined move/switch command", ops)
	}
}

func TestAttachSingletonSidebarAndSwitchClientSameWindowMarksOwnerAfterSwitch(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{
		"resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-1",
		";", "select-pane", "-t", "%9", "-R",
	}).Return(ports.Result{}, nil)

	if err := (Client{Process: process}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20"); err != nil {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error: %v", err)
	}
}

func TestAttachSingletonSidebarAndSwitchClientSameWindowReturnsSwitchChainFailure(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	boom := errors.New("switch failed")

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{
		"resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-pane", "1",
		";", "set-option", "-p", "-t", "%9", "@session-sidebar-owner-client", "client-1",
		";", "select-pane", "-t", "%9", "-R",
	}).Return(ports.Result{Stderr: "can't find client\n"}, boom)

	err := (Client{Process: process}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error = %v, want %v", err, boom)
	}
}

func TestAttachSingletonSidebarAndSwitchClientKeepsSourceLayoutWhenCombinedCommandFailsBeforeSidebarMoves(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("join failed")
	rec := newRecPort(t)

	paneWindowID := "@1"
	restoredTargetLayout := false

	rec.handle([]string{"show-options", "-pv", "-t", "%9", optionSidebarPane}, func([]string) (string, string) {
		return "1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}, func([]string) (string, string) {
		return "@2", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_id}"}, func([]string) (string, string) {
		return "@1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "%9", "#{window_id}"}, func([]string) (string, string) {
		return paneWindowID, ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%9\t0\t0\t30\t48\t1\n%27\t31\t0\t74\t48\t0\n%185\t106\t0\t75\t48\t0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@2", "#{window_layout}"}, func([]string) (string, string) {
		return "layout-before-sidebar", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@2", optionSidebarWindowLayout, "layout-before-sidebar"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handleErr([]string{
		"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", optionSidebarPane, "1",
		";", "set-option", "-p", "-t", "%9", optionSidebarOwnerClient, "client-1",
		";", "resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "select-pane", "-t", "%9", "-R",
	}, func([]string) (string, string) {
		return "", "can't join pane\n"
	}, boom)
	rec.handle([]string{"resize-pane", "-t", "%9", "-x", "20"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"select-pane", "-t", "%9"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"show-options", "-w", "-v", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "layout-before-sidebar", ""
	})
	rec.handle([]string{"select-layout", "-t", "@2", "layout-before-sidebar"}, func([]string) (string, string) {
		restoredTargetLayout = true
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "", ""
	})

	err := (Client{Process: rec}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error = %v, want %v", err, boom)
	}
	if !restoredTargetLayout {
		t.Fatalf("expected rollback to restore the target layout; calls: %#v", rec.calls)
	}
	for _, call := range rec.calls {
		if len(call) == 5 && call[0] == "set-option" && call[1] == "-wu" && call[2] == "-t" && call[3] == "@1" && call[4] == optionSidebarWindowLayout {
			t.Fatalf("source layout should stay intact when the sidebar never left the source window; calls: %#v", rec.calls)
		}
	}
}

func TestAttachSingletonSidebarAndSwitchClientRollsBackWhenCombinedCommandFails(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("switch failed")
	rec := newRecPort(t)

	paneWindowID := "@1"
	savedLayouts := map[string]string{}
	restoredTargetLayout := false
	rebalancedSource := false

	rec.handle([]string{"show-options", "-pv", "-t", "%9", optionSidebarPane}, func([]string) (string, string) {
		return "1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}, func([]string) (string, string) {
		return "@2", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_id}"}, func([]string) (string, string) {
		return "@1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "%9", "#{window_id}"}, func([]string) (string, string) {
		return paneWindowID, ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		if paneWindowID == "@1" {
			return "%9\t0\t0\t30\t48\t1\n%27\t31\t0\t74\t48\t0\n%185\t106\t0\t75\t48\t0", ""
		}
		return "%27\t0\t0\t105\t48\t0\n%185\t106\t0\t75\t48\t0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@2", "#{window_layout}"}, func([]string) (string, string) {
		return "layout-before-sidebar", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_layout}"}, func([]string) (string, string) {
		return "source-layout", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@2", optionSidebarWindowLayout, "layout-before-sidebar"}, func([]string) (string, string) {
		savedLayouts["@2"] = "layout-before-sidebar"
		return "", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@1", optionSidebarWindowLayout, "source-layout"}, func([]string) (string, string) {
		savedLayouts["@1"] = "source-layout"
		return "", ""
	})
	rec.handleErr([]string{
		"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", optionSidebarPane, "1",
		";", "set-option", "-p", "-t", "%9", optionSidebarOwnerClient, "client-1",
		";", "resize-pane", "-t", "%9", "-x", "20",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "select-pane", "-t", "%9", "-R",
	}, func([]string) (string, string) {
		paneWindowID = "@2"
		return "", "can't find client\n"
	}, boom)
	rec.handle([]string{"join-pane", "-hbf", "-d", "-l", "20", "-s", "%9", "-t", "@1"}, func([]string) (string, string) {
		paneWindowID = "@1"
		return "", ""
	})
	rec.handle([]string{"set-option", "-p", "-t", "%9", optionSidebarPane, "1"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%27", "-x", "90"}, func([]string) (string, string) {
		rebalancedSource = true
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%9", "-x", "20"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"select-pane", "-t", "%9"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"show-options", "-w", "-v", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		return savedLayouts["@2"], ""
	})
	rec.handle([]string{"select-layout", "-t", "@2", "layout-before-sidebar"}, func([]string) (string, string) {
		restoredTargetLayout = true
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		delete(savedLayouts, "@2")
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout}, func([]string) (string, string) {
		delete(savedLayouts, "@1")
		return "", ""
	})

	err := (Client{Process: rec}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "20")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error = %v, want %v", err, boom)
	}
	if !restoredTargetLayout {
		t.Fatalf("expected rollback to restore target layout before clearing it; calls: %#v", rec.calls)
	}
	if !rebalancedSource {
		t.Fatalf("expected rollback to rebalance the source window before restoring the sidebar; calls: %#v", rec.calls)
	}
}

func TestAttachSingletonSidebarRollbackRebalancesSourceWindowAfterPostJoinFailure(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("select failed")
	rec := newRecPort(t)

	paneWindowID := "@1"
	rebalancedSource := false
	restoredTargetLayout := false

	rec.handle([]string{"show-options", "-pv", "-t", "%9", optionSidebarPane}, func([]string) (string, string) {
		return "1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "client-1", "#{window_id}"}, func([]string) (string, string) {
		return "@2", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "%9", "#{window_id}"}, func([]string) (string, string) {
		return paneWindowID, ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		if paneWindowID == "@1" {
			return "%9\t0\t0\t30\t48\t1\n%27\t31\t0\t74\t48\t0\n%185\t106\t0\t75\t48\t0", ""
		}
		return "%27\t0\t0\t105\t48\t0\n%185\t106\t0\t75\t48\t0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@2", "#{window_layout}"}, func([]string) (string, string) {
		return "target-layout-before-sidebar", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@2", optionSidebarWindowLayout, "target-layout-before-sidebar"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"join-pane", "-hbf", "-d", "-l", "30", "-s", "%9", "-t", "@2"}, func([]string) (string, string) {
		paneWindowID = "@2"
		return "", ""
	})
	rec.handle([]string{"set-option", "-p", "-t", "%9", optionSidebarPane, "1"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%9", "-x", "30"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handleErr([]string{"select-pane", "-t", "%9"}, func([]string) (string, string) {
		return "", "select failed\n"
	}, boom)
	rec.handle([]string{"resize-pane", "-t", "%27", "-x", "90"}, func([]string) (string, string) {
		rebalancedSource = true
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"join-pane", "-hbf", "-d", "-l", "30", "-s", "%9", "-t", "@1"}, func([]string) (string, string) {
		paneWindowID = "@1"
		return "", ""
	})
	rec.handle([]string{"show-options", "-w", "-v", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "target-layout-before-sidebar", ""
	})
	rec.handle([]string{"select-layout", "-t", "@2", "target-layout-before-sidebar"}, func([]string) (string, string) {
		restoredTargetLayout = true
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@2", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "", ""
	})

	_, err := (Client{Process: rec}).AttachSingletonSidebar(ctx, "client-1", "%9", "30")
	if !errors.Is(err, boom) {
		t.Fatalf("AttachSingletonSidebar error = %v, want %v", err, boom)
	}
	if !rebalancedSource {
		t.Fatalf("expected rollback to rebalance the source window before restoring the sidebar; calls: %#v", rec.calls)
	}
	if !restoredTargetLayout {
		t.Fatalf("expected rollback to restore the target layout after moving the sidebar back; calls: %#v", rec.calls)
	}
}

func TestAttachSingletonSidebarAndSwitchClientRebalancesSourceWindowAfterSuccess(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)

	paneWindowID := "@1"
	rebalancedSource := false
	capturedTargetBaseline := false

	rec.handle([]string{"show-options", "-pv", "-t", "%9", optionSidebarPane}, func([]string) (string, string) {
		return "1", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "=beta:", "#{window_id}"}, func([]string) (string, string) {
		return "@2", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "%9", "#{window_id}"}, func([]string) (string, string) {
		return paneWindowID, ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		if paneWindowID == "@1" {
			return "%9\t0\t0\t30\t48\t1\n%27\t31\t0\t74\t48\t0\n%185\t106\t0\t75\t48\t0", ""
		}
		return "%27\t0\t0\t105\t48\t0\n%185\t106\t0\t75\t48\t0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@2", "#{window_layout}"}, func([]string) (string, string) {
		return "layout-before-sidebar", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@2", optionSidebarWindowLayout, "layout-before-sidebar"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{
		"join-pane", "-hbf", "-d", "-l", "30", "-s", "%9", "-t", "@2",
		";", "set-option", "-p", "-t", "%9", optionSidebarPane, "1",
		";", "set-option", "-p", "-t", "%9", optionSidebarOwnerClient, "client-1",
		";", "resize-pane", "-t", "%9", "-x", "30",
		";", "switch-client", "-c", "client-1", "-t", "=beta:",
		";", "select-pane", "-t", "%9", "-R",
	}, func([]string) (string, string) {
		paneWindowID = "@2"
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%27", "-x", "90"}, func([]string) (string, string) {
		rebalancedSource = true
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"show-options", "-w", "-v", "-t", "@2", optionSidebarResizeSyncActive}, func([]string) (string, string) {
		return "0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@2", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@2", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%9\t0\t0\t30\t48\t1\n%301\t31\t0\t74\t48\t0\n%302\t106\t0\t75\t48\t0", ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@2", optionSidebarOpenWorkBaseline, `{"representativePaneIDs":["%301","%302"],"workWidths":[74,75]}`}, func([]string) (string, string) {
		capturedTargetBaseline = true
		return "", ""
	})

	if err := (Client{Process: rec}).AttachSingletonSidebarAndSwitchClient(ctx, "client-1", "beta", "%9", "30"); err != nil {
		t.Fatalf("AttachSingletonSidebarAndSwitchClient error: %v", err)
	}
	if !rebalancedSource {
		t.Fatalf("expected successful move-and-switch to rebalance the source window; calls: %#v", rec.calls)
	}
	if !capturedTargetBaseline {
		t.Fatalf("expected successful move-and-switch to capture the target sidebar-open baseline; calls: %#v", rec.calls)
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
				// display-message for WindowID is called twice: once before break-pane
				// (validate pane exists -> @1) and once after (resolve parked window -> @parked).
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}).Return(ports.Result{Stdout: "100\t30\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}).Return(ports.Result{Stdout: "%9\t0\t0\t100\t30\t1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{Stderr: "can't find session\n"}, errors.New("missing"))
				process.EXPECT().Exec(ctx, "tmux", []string{"new-session", "-d", "-s", "__tmux-session-sidebar", "-n", "sidebar-parked"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
				// The stale saved hidden-layout option is cleared best-effort after break-pane.
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil)
			},
		},
		{
			name: "cleans stale parking windows after parking marked pane",
			setup: func(ctx context.Context, process *mocks.MockProcessPort) {
				process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
				// display-message for WindowID is called twice: once before break-pane
				// (validate pane exists -> @1) and once after (resolve parked window -> @parked).
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}).Return(ports.Result{Stdout: "100\t30\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}).Return(ports.Result{Stdout: "%9\t0\t0\t100\t30\t1\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil).Once()
				process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
				// The stale saved hidden-layout option is cleared best-effort after break-pane.
				process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@stale\n@parked\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"}).Return(ports.Result{Stdout: "%9\t@parked\t0\n"}, nil)
				process.EXPECT().Exec(ctx, "tmux", []string{"kill-window", "-t", "@stale"}).Return(ports.Result{}, nil)
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

func TestParkSingletonSidebarSucceedsWithoutHiddenLayoutRestore(t *testing.T) {
	// Park does not save a visible-layout snapshot or restore a hidden layout.
	// Once break-pane removes the sidebar, tmux's live layout is authoritative.
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-pv", "-t", "%9", "@session-sidebar-pane"}).Return(ports.Result{Stdout: "1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"}).Return(ports.Result{Stdout: "100\t30\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane}).Return(ports.Result{Stdout: "%9\t0\t0\t100\t30\t1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"has-session", "-t", "__tmux-session-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"break-pane", "-d", "-s", "%9", "-t", "__tmux-session-sidebar:"}).Return(ports.Result{}, nil)
	// The stale saved hidden-layout option is cleared best-effort after break-pane.
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "%9", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-windows", "-t", "__tmux-session-sidebar", "-F", "#{window_id}"}).Return(ports.Result{Stdout: "@parked\n"}, nil)

	if err := (Client{Process: process}).ParkSingletonSidebar(ctx, "%9"); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
}

func TestSaveAndRestoreWindowLayout(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", "#{window_layout}"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wq", "-t", "@1", "@session-sidebar-window-layout", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	if err := client.saveTargetWindowLayoutBeforeAttach(ctx, "@1"); err != nil {
		t.Fatalf("saveTargetWindowLayoutBeforeAttach error: %v", err)
	}

	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{Stdout: "layout-before-sidebar\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"select-layout", "-t", "@1", "layout-before-sidebar"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", "@session-sidebar-window-layout"}).Return(ports.Result{}, nil)
	if err := client.RestoreWindowLayout(ctx, "@1"); err != nil {
		t.Fatalf("RestoreWindowLayout error: %v", err)
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
	// No visible-layout snapshot is saved and no select-layout replay occurs.
	// The background cleanup waits for pane disappearance, then clears the
	// stale saved hidden-layout option.
	process.EXPECT().Exec(ctx, "tmux", mock.MatchedBy(func(args []string) bool {
		if len(args) != 3 || args[0] != "run-shell" || args[1] != "-b" {
			return false
		}
		command := args[2]
		for _, want := range []string{"list-panes", "##{pane_id}", "set-option", "@session-sidebar-window-layout", "%9", "@1"} {
			if !strings.Contains(command, want) {
				return false
			}
		}
		return !strings.Contains(command, "select-layout") && !strings.Contains(command, "show-options")
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

func TestScheduleSidebarRestoreOnExitClearsSavedLayoutWhenSidebarPaneIsAlreadyGone(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "client-1", "#{window_id}"}).Return(ports.Result{Stdout: "@1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatPaneID + "\t" + formatSidebarPane + "\t#{pane_dead}"}).Return(ports.Result{Stdout: "%2\t0\t0\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout}).Return(ports.Result{}, nil)

	if err := client.ScheduleSidebarRestoreOnExit(ctx, "client-1", ""); err != nil {
		t.Fatalf("ScheduleSidebarRestoreOnExit error: %v", err)
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

func TestRefreshAllSidebarsSendsF5ToMarkedPanes(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}
	process.EXPECT().Exec(ctx, "tmux", []string{cmdListPanes, "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", formatPaneID}).Return(ports.Result{Stdout: "%1\n%2\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{cmdSendKeys, "-t", "%1", "F5"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{cmdSendKeys, "-t", "%2", "F5"}).Return(ports.Result{}, nil)

	if err := client.RefreshAllSidebars(ctx); err != nil {
		t.Fatalf("RefreshAllSidebars error: %v", err)
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

// ─── Repeated park cycle window cleanup (custom mock) ──────────────────

// recProcessPort is a simple inline tmux mock that uses a slice-based
// handler list to avoid testify mock ordering issues.
type recProcessPort struct {
	t        *testing.T
	calls    [][]string
	handlers []recHandler
}

type recHandler struct {
	pattern []string
	fn      func(args []string) (string, string)
	withErr error
}

func newRecPort(t *testing.T) *recProcessPort {
	return &recProcessPort{t: t}
}

func (r *recProcessPort) Exec(_ context.Context, name string, args []string) (ports.Result, error) {
	if name != "tmux" {
		r.t.Fatalf("unexpected command %q", name)
	}
	r.calls = append(r.calls, args)
	for _, h := range r.handlers {
		if len(args) != len(h.pattern) {
			continue
		}
		match := true
		for i, p := range h.pattern {
			if p == "*" {
				continue
			}
			if args[i] != p {
				match = false
				break
			}
		}
		if match {
			stdout, stderr := h.fn(args)
			if h.withErr != nil {
				return ports.Result{Stdout: stdout, Stderr: stderr}, h.withErr
			}
			return ports.Result{Stdout: stdout, Stderr: stderr}, nil
		}
	}
	r.t.Fatalf("unexpected tmux args: %#v", args)
	return ports.Result{}, nil
}

func (r *recProcessPort) handle(args []string, fn func([]string) (string, string)) {
	r.handlers = append(r.handlers, recHandler{pattern: args, fn: fn})
}

func (r *recProcessPort) handleErr(args []string, fn func([]string) (string, string), err error) {
	r.handlers = append(r.handlers, recHandler{pattern: args, fn: fn, withErr: err})
}

// TestParkSingletonSidebarKillsStaleWindows verifies that parking the sidebar
// twice triggers cleanupParkingWindows which kills stale windows from the
// first park but preserves the current parked window.
func TestParkSingletonSidebarKillsStaleWindows(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)

	paneID := "%9"

	// ---- First park ----
	paneLookupCount := 0
	rec.handle(
		[]string{"display-message", "-p", "-t", paneID, "#{window_id}"},
		func(args []string) (string, string) {
			paneLookupCount++
			if paneLookupCount == 1 {
				return "@1", ""
			}
			return "@parked", ""
		},
	)
	rec.handle([]string{"show-options", "-pv", "-t", paneID, optionSidebarPane},
		func(args []string) (string, string) { return "1", "" })
	rec.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"},
		func(args []string) (string, string) { return "100\t30", "" })
	rec.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane},
		func(args []string) (string, string) { return paneID + "\t0\t0\t100\t30\t1", "" })
	rec.handle([]string{"has-session", "-t", singletonSidebarSessionName},
		func(args []string) (string, string) { return "", "" })
	rec.handle([]string{"break-pane", "-d", "-s", paneID, "-t", singletonSidebarSessionName + ":"},
		func(args []string) (string, string) { return "", "" })
	// The stale saved hidden-layout option is cleared best-effort after break-pane.
	rec.handle([]string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout},
		func(args []string) (string, string) { return "", "" })
	rec.handle([]string{"list-windows", "-t", singletonSidebarSessionName, "-F", "#{window_id}"},
		func(args []string) (string, string) { return "@parked", "" })

	client := Client{Process: rec}
	if err := client.ParkSingletonSidebar(ctx, paneID); err != nil {
		t.Fatalf("first park error: %v", err)
	}

	// ---- Second park: list-windows now includes a stale window ----
	client2 := Client{Process: newRecPort(t)}
	rec2 := client2.Process.(*recProcessPort)
	secondPaneLookupCount := 0
	rec2.handle(
		[]string{"display-message", "-p", "-t", paneID, "#{window_id}"},
		func(args []string) (string, string) {
			secondPaneLookupCount++
			if secondPaneLookupCount == 1 {
				return "@1", ""
			}
			return "@parked", ""
		},
	)
	rec2.handle([]string{"show-options", "-pv", "-t", paneID, optionSidebarPane},
		func(args []string) (string, string) { return "1", "" })
	rec2.handle([]string{"display-message", "-p", "-t", "@1", "#{window_width}\t#{window_height}"},
		func(args []string) (string, string) { return "100\t30", "" })
	rec2.handle([]string{"list-panes", "-t", "@1", "-F", formatSidebarRebalancePane},
		func(args []string) (string, string) { return paneID + "\t0\t0\t100\t30\t1", "" })
	rec2.handle([]string{"has-session", "-t", singletonSidebarSessionName},
		func(args []string) (string, string) { return "", "" })
	rec2.handle([]string{"break-pane", "-d", "-s", paneID, "-t", singletonSidebarSessionName + ":"},
		func(args []string) (string, string) { return "", "" })
	// The stale saved hidden-layout option is cleared best-effort after break-pane.
	rec2.handle([]string{"set-option", "-wu", "-t", "@1", optionSidebarWindowLayout},
		func(args []string) (string, string) { return "", "" })
	rec2.handle([]string{"list-windows", "-t", singletonSidebarSessionName, "-F", "#{window_id}"},
		func(args []string) (string, string) { return "@stale\n@parked", "" })
	rec2.handle([]string{"list-panes", "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"},
		func(args []string) (string, string) { return "%9\t@parked\t0", "" })
	rec2.handle([]string{"kill-window", "-t", "@stale"},
		func(args []string) (string, string) { return "", "" })

	if err := client2.ParkSingletonSidebar(ctx, paneID); err != nil {
		t.Fatalf("second park error: %v", err)
	}

	// Verify kill-window was called for the stale window
	foundKill := false
	for _, call := range rec2.calls {
		if len(call) >= 3 && call[0] == "kill-window" && call[2] == "@stale" {
			foundKill = true
			break
		}
	}
	if !foundKill {
		t.Errorf("kill-window @stale was not called during second park (got %d calls)", len(rec2.calls))
		for i, c := range rec2.calls {
			t.Logf("  call %d: %v", i, c)
		}
	}
}

func TestCleanupParkingWindowsPreservesMarkedSidebarWindows(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)
	rec.handle([]string{"list-windows", "-t", singletonSidebarSessionName, "-F", "#{window_id}"},
		func(args []string) (string, string) { return "@oldSidebar\n@stale\n@current", "" })
	rec.handle([]string{"list-panes", "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"},
		func(args []string) (string, string) { return "%old\t@oldSidebar\t0\n%current\t@current\t0", "" })
	rec.handle([]string{"kill-window", "-t", "@stale"},
		func(args []string) (string, string) { return "", "" })

	(Client{Process: rec}).cleanupParkingWindows(ctx, "@current")

	killed := killedWindows(rec.calls)
	if killed["@oldSidebar"] || !killed["@stale"] || killed["@current"] {
		t.Fatalf("killed windows = %#v, want only @stale", killed)
	}
}

func TestCleanupParkingWindowsPreservesAllWindowsWhenMarkedPaneDiscoveryFails(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)
	rec.handle([]string{"list-windows", "-t", singletonSidebarSessionName, "-F", "#{window_id}"},
		func(args []string) (string, string) { return "@oldSidebar\n@stale\n@current", "" })
	rec.handleErr([]string{"list-panes", "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"},
		func(args []string) (string, string) { return "", "tmux unavailable" }, errors.New("list panes failed"))

	(Client{Process: rec}).cleanupParkingWindows(ctx, "@current")

	if killed := killedWindows(rec.calls); len(killed) != 0 {
		t.Fatalf("killed windows = %#v, want fail-closed cleanup with no kills", killed)
	}
}

func killedWindows(calls [][]string) map[string]bool {
	killed := map[string]bool{}
	for _, call := range calls {
		if len(call) == 3 && call[0] == "kill-window" && call[1] == "-t" {
			killed[call[2]] = true
		}
	}
	return killed
}

// TestFindSingletonSidebarCleansUpDeadPaneReferences verifies that FindSingletonSidebar
// kills dead marked panes before returning, preventing stale-pane accumulation.
func TestFindSingletonSidebarCleansUpDeadPaneReferences(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)

	rec.handle([]string{"list-panes", "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}"},
		func(args []string) (string, string) {
			return "%dead\t@gone\t1\n%live\t@active\t0", ""
		})
	rec.handle([]string{"kill-pane", "-t", "%dead"},
		func(args []string) (string, string) { return "", "" })

	ref, err := (Client{Process: rec}).FindSingletonSidebar(ctx)
	if err != nil {
		t.Fatalf("FindSingletonSidebar error: %v", err)
	}
	if ref.PaneID != "%live" || ref.WindowID != "@active" {
		t.Fatalf("FindSingletonSidebar = %#v, want %#v", ref, ports.PaneRef{PaneID: "%live", WindowID: "@active"})
	}
	foundKill := false
	for _, call := range rec.calls {
		if len(call) == 3 && call[0] == "kill-pane" && call[1] == "-t" && call[2] == "%dead" {
			foundKill = true
			break
		}
	}
	if !foundKill {
		t.Fatalf("FindSingletonSidebar did not kill the dead pane; calls=%#v", rec.calls)
	}
}

func assertSessions(t *testing.T, got []ports.SessionSnapshot, want []ports.SessionSnapshot) {
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

func assertClients(t *testing.T, got []ports.ClientSnapshot, want []ports.ClientSnapshot) {
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

func TestSessionMetadataRoundTripsLastPathCompatibilityOption(t *testing.T) {
	ctx := context.Background()
	process := mocks.NewMockProcessPort(t)
	client := Client{Process: process}
	metadata := ports.SessionMetadata{Kind: "adhoc", ProjectPath: "/tmp/project", LastPath: "/tmp/last"}

	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-t", "scratch", "@session-sidebar-kind", "adhoc"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-t", "scratch", "@session-sidebar-project-path", "/tmp/project"}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"set-option", "-t", "scratch", "@session-sidebar-last-path", "/tmp/last"}).Return(ports.Result{}, nil)
	if err := client.SaveSessionMetadata(ctx, "scratch", metadata); err != nil {
		t.Fatalf("SaveSessionMetadata error: %v", err)
	}

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "scratch", "#{@session-sidebar-kind}"}).Return(ports.Result{Stdout: "adhoc\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "scratch", "#{@session-sidebar-project-path}"}).Return(ports.Result{Stdout: "/tmp/project\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "scratch", "#{@session-sidebar-last-path}"}).Return(ports.Result{Stdout: "/tmp/last\n"}, nil)
	got, err := client.LoadSessionMetadata(ctx, "scratch")
	if err != nil {
		t.Fatalf("LoadSessionMetadata error: %v", err)
	}
	if got != metadata {
		t.Fatalf("LoadSessionMetadata() = %#v, want %#v", got, metadata)
	}
}
