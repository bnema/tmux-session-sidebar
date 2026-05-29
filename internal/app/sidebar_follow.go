package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func withSidebarFollow(ctx context.Context, client string, sidebar ports.TmuxSidebarPort, action func() error) error {
	if action == nil {
		return nil
	}
	if client == "" {
		return action()
	}
	shouldFollow, err := sidebarShouldBeVisibleForClient(ctx, client)
	if err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	if !shouldFollow || sidebar == nil {
		return nil
	}
	return applySidebarVisibilityForClient(ctx, client, sidebar)
}

func reconcileSidebarVisibilityForClient(ctx context.Context, client string, sidebar ports.TmuxSidebarPort) error {
	if strings.TrimSpace(client) == "" || sidebar == nil {
		return nil
	}
	shouldFollow, err := sidebarShouldBeVisibleForClient(ctx, client)
	if err != nil {
		return err
	}
	if !shouldFollow {
		return nil
	}
	return applySidebarVisibilityForClient(ctx, client, sidebar)
}

func applySidebarVisibilityForClient(ctx context.Context, client string, sidebar ports.TmuxSidebarPort) error {
	if closeAfterSwitch(ctx, sidebar) {
		return closeSidebar(ctx, sidebar)
	}
	return openSidebarForClient(ctx, client, "", "", sidebar)
}

func closeAfterSwitch(ctx context.Context, sidebar ports.TmuxSidebarPort) bool {
	shouldClose, err := sidebar.CloseAfterSwitch(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: read close-after-switch failed: %v\n", err)
		return false
	}
	return shouldClose
}
