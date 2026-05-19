package app

import (
	"context"
	"fmt"
	"os"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func withSidebarFollow(ctx context.Context, client string, sidebar ports.TmuxSidebarPort, action func() error) error {
	if action == nil {
		return nil
	}
	if client == "" {
		return action()
	}

	oldPane, err := existingSidebarPane(ctx, client, sidebar)
	if err != nil {
		return err
	}
	wasOpen := oldPane != ""
	if err := action(); err != nil {
		return err
	}
	if !wasOpen {
		return nil
	}
	if closeAfterSwitch(ctx, sidebar) {
		return sidebar.CloseSidebarPane(ctx, oldPane)
	}
	targetPane, err := existingSidebarPane(ctx, client, sidebar)
	if err != nil {
		return err
	}
	if targetPane != "" {
		return sidebar.RefreshSidebar(ctx, client)
	}
	return openSidebar(ctx, map[string]string{"client": client}, sidebar)
}

func closeAfterSwitch(ctx context.Context, sidebar ports.TmuxSidebarPort) bool {
	shouldClose, err := sidebar.CloseAfterSwitch(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: read close-after-switch failed: %v\n", err)
		return false
	}
	return shouldClose
}
