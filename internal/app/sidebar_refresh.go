package app

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func refreshAllSidebarPanes(ctx context.Context) error {
	output, err := tmux(ctx, "list-panes", "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}")
	if err != nil {
		return tmuxCommandError("list sidebar panes", output, err)
	}
	var firstErr error
	for paneID := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		paneID = strings.TrimSpace(paneID)
		if paneID == "" {
			continue
		}
		sendOutput, sendErr := tmux(ctx, "send-keys", "-t", paneID, "F5")
		if sendErr != nil {
			if firstErr == nil {
				firstErr = tmuxCommandError("refresh sidebar pane", sendOutput, sendErr)
			}
			continue
		}
	}
	return firstErr
}

func refreshAllSidebarPanesBestEffort(ctx context.Context) {
	if err := refreshAllSidebarPanes(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: refresh all sidebars failed: %v\n", err)
	}
}
