package tmuxcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestSidebarDebugSnapshotIncludesPaneSizesAndLayout(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\t140\t40\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "%1\t0\t120\t40\t0\t0\t1\t0\tzsh\t0\n%10\t1\t20\t40\t120\t0\t0\t0\ttmux-session-sidebar\t1\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarOpenWorkBaseline}).Return(ports.Result{}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarResizeSyncActive}).Return(ports.Result{}, nil)

	got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
	if err != nil {
		t.Fatalf("SidebarDebugSnapshot error: %v", err)
	}
	for _, want := range []string{
		"session=work($1)",
		"window=@1:1:editor",
		"size=140x40",
		"layout=main-vertical",
		"baseline=none",
		"sync=false",
		`%1[idx=0,size=120x40,pos=0,0,active=true,dead=false,sidebar=false,cmd=zsh]`,
		`%10[idx=1,size=20x40,pos=120,0,active=false,dead=false,sidebar=true,cmd=tmux-session-sidebar]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SidebarDebugSnapshot = %q, want substring %q", got, want)
		}
	}
}

func TestSidebarDebugSnapshotIncludesSavedBaselineAndSyncFlag(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\t181\t48\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "%9\t0\t30\t48\t0\t0\t0\t0\ttmux-session-sidebar\t1\n%1\t1\t74\t48\t31\t0\t1\t0\tzsh\t0\n%2\t2\t75\t48\t106\t0\t0\t0\tvim\t0\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarOpenWorkBaseline}).Return(ports.Result{Stdout: `{"representativePaneIDs":["%1","%2"],"workWidths":[74,75]}` + "\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarResizeSyncActive}).Return(ports.Result{Stdout: "1\n"}, nil)

	got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
	if err != nil {
		t.Fatalf("SidebarDebugSnapshot error: %v", err)
	}
	for _, want := range []string{`baseline=%1\=74\,%2\=75`, "sync=true", "size=181x48"} {
		if !strings.Contains(got, want) {
			t.Fatalf("SidebarDebugSnapshot = %q, want substring %q", got, want)
		}
	}
}

func TestSidebarDebugSnapshotHandlesEdgeCases(t *testing.T) {
	t.Run("empty window id is a no-op", func(t *testing.T) {
		got, err := (Client{}).SidebarDebugSnapshot(t.Context(), "   ")
		if err != nil {
			t.Fatalf("SidebarDebugSnapshot error = %v, want nil", err)
		}
		if got != "" {
			t.Fatalf("SidebarDebugSnapshot = %q, want empty", got)
		}
	})

	t.Run("propagates display-message error", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		boom := newTestError()
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stderr: "no such window\n"}, boom)

		_, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
		if !errors.Is(err, boom) {
			t.Fatalf("SidebarDebugSnapshot error = %v, want %v", err, boom)
		}
	})

	t.Run("propagates list-panes error", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		boom := newTestError()
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\t140\t40\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stderr: "list-panes failed\n"}, boom)

		_, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
		if !errors.Is(err, boom) {
			t.Fatalf("SidebarDebugSnapshot error = %v, want %v", err, boom)
		}
	})

	t.Run("formats windows with no panes", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\t140\t40\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarOpenWorkBaseline}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarResizeSyncActive}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
		if err != nil {
			t.Fatalf("SidebarDebugSnapshot error: %v", err)
		}
		if !strings.Contains(got, "panes=none") {
			t.Fatalf("SidebarDebugSnapshot = %q, want panes=none", got)
		}
	})

	t.Run("sanitizes special characters in snapshot values", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "proj;ect\t$1\t@1\t1\tedit[or]\"q\"\tmain-vertical\t140\t40\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "%1\t0\t120\t40\t0\t0\t1\t0\tcmd;with\\slashes\t0\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarOpenWorkBaseline}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarResizeSyncActive}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
		if err != nil {
			t.Fatalf("SidebarDebugSnapshot error: %v", err)
		}
		for _, want := range []string{`session=proj\;ect($1)`, `window=@1:1:edit[or]"q"`, `cmd=cmd\;with\\slashes`} {
			if !strings.Contains(got, want) {
				t.Fatalf("SidebarDebugSnapshot = %q, want substring %q", got, want)
			}
		}
	})

	t.Run("malformed pane rows degrade to placeholders", func(t *testing.T) {
		ctx := t.Context()
		process := mocks.NewMockProcessPort(t)
		process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\t140\t40\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "\n%1\n\n"}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarOpenWorkBaseline}).Return(ports.Result{}, nil)
		process.EXPECT().Exec(ctx, "tmux", []string{"show-options", "-w", "-v", "-t", "@1", optionSidebarResizeSyncActive}).Return(ports.Result{}, nil)

		got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
		if err != nil {
			t.Fatalf("SidebarDebugSnapshot error: %v", err)
		}
		if !strings.Contains(got, `%1[idx=-,size=-x-,pos=-,-,active=false,dead=false,sidebar=false,cmd=-]`) {
			t.Fatalf("SidebarDebugSnapshot = %q, want placeholder pane summary", got)
		}
	})
}

func newTestError() error {
	return errors.New("boom")
}
