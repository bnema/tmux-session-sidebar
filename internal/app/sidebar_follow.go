package app

import (
	"context"

	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func withSidebarFollow(ctx context.Context, client string, action func() error) error {
	if action == nil {
		return nil
	}
	if client == "" {
		return action()
	}

	oldPane, _ := existingSidebarPane(ctx, client)
	wasOpen := oldPane != ""
	if err := action(); err != nil {
		return err
	}
	if !wasOpen {
		return nil
	}
	if closeAfterSwitch(ctx) {
		_, err := tmux(ctx, "kill-pane", "-t", oldPane)
		return err
	}
	if err := openSidebar(ctx, map[string]string{"client": client}); err != nil {
		return err
	}
	_, err := tmux(ctx, "kill-pane", "-t", oldPane)
	return err
}

func closeAfterSwitch(ctx context.Context) bool {
	cfg, err := loadTmuxConfig(ctx)
	if err != nil {
		return false
	}
	return cfg.CloseAfterSwitch
}

var loadTmuxConfig = func(ctx context.Context) (ports.ConfigSnapshot, error) {
	return (tmuxcli.Client{Process: process.Runner{}}).LoadConfig(ctx)
}
