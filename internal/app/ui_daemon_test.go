package app

import (
	"context"
	"io"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestServeSidebarUIRunsUI(t *testing.T) {
	uiRan := false
	oldRunSidebarUI := runSidebarUI
	defer func() { runSidebarUI = oldRunSidebarUI }()
	runSidebarUI = func(context.Context, map[string]string, io.Writer, ports.TmuxSidebarPort) error {
		uiRan = true
		return nil
	}

	err := serveSidebarUI(t.Context(), map[string]string{"client": "%1"}, io.Discard, nil)
	if err != nil {
		t.Fatalf("serveSidebarUI error: %v", err)
	}
	if !uiRan {
		t.Fatal("runSidebarUI was not called")
	}
}
