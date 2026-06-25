package app

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func adoptPersistedOpenSidebar(ctx context.Context, client string, sidebar ports.SidebarPort) error {
	client = strings.TrimSpace(client)
	if client == "" || sidebar == nil {
		return nil
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		return err
	}
	if !sidebarStateAppliesToClient(state, client) {
		return nil
	}
	pane, err := sidebar.FindSidebarPane(ctx, client)
	if err != nil {
		return err
	}
	if pane.PaneID != "" {
		return nil
	}
	return applySidebarVisibilityForClient(ctx, client, sidebar)
}
