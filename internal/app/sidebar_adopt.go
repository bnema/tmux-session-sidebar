package app

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func adoptPersistedOpenSidebar(ctx context.Context, client string, sidebar ports.TmuxSidebarPort) error {
	client = strings.TrimSpace(client)
	if client == "" || sidebar == nil {
		return nil
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		return err
	}
	if !state.Open {
		return nil
	}
	pane, err := sidebar.FindSidebarPane(ctx, client)
	if err != nil {
		return err
	}
	if pane.PaneID != "" {
		if state.OwnerClient != client {
			return saveSidebarVisibility(ctx, true, client)
		}
		return nil
	}
	singleton, err := sidebar.FindSingletonSidebar(ctx)
	if err != nil {
		return err
	}
	if singleton.PaneID != "" {
		return nil
	}
	if err := saveSidebarVisibility(ctx, true, client); err != nil {
		return err
	}
	return applySidebarVisibilityForClient(ctx, client, sidebar)
}
