package app

import (
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
