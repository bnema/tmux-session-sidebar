package tmuxcli

import (
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestSidebarDebugSnapshotIncludesPaneSizesAndLayout(t *testing.T) {
	ctx := t.Context()
	process := mocks.NewMockProcessPort(t)

	process.EXPECT().Exec(ctx, "tmux", []string{"display-message", "-p", "-t", "@1", formatSidebarDebugWindow}).Return(ports.Result{Stdout: "work\t$1\t@1\t1\teditor\tmain-vertical\n"}, nil)
	process.EXPECT().Exec(ctx, "tmux", []string{"list-panes", "-t", "@1", "-F", formatSidebarDebugPane}).Return(ports.Result{Stdout: "%1\t0\t120\t40\t0\t0\t1\t0\tzsh\t0\n%10\t1\t20\t40\t120\t0\t0\t0\ttmux-session-sidebar\t1\n"}, nil)

	got, err := (Client{Process: process}).SidebarDebugSnapshot(ctx, "@1")
	if err != nil {
		t.Fatalf("SidebarDebugSnapshot error: %v", err)
	}
	for _, want := range []string{
		"session=work($1)",
		"window=@1:1:editor",
		"layout=main-vertical",
		"%1[idx=0,size=120x40,pos=0,0,active=true,dead=false,sidebar=false,cmd=zsh]",
		"%10[idx=1,size=20x40,pos=120,0,active=false,dead=false,sidebar=true,cmd=tmux-session-sidebar]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SidebarDebugSnapshot = %q, want substring %q", got, want)
		}
	}
}
