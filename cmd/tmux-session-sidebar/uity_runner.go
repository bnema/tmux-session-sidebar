package main

import (
	"context"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/internal/app"
	"github.com/bnema/tmux-session-sidebar/internal/viewmodel"
)

type uiRunner struct{}

func (uiRunner) Run(ctx context.Context, items []viewmodel.TreeItem, actions app.SidebarUIActions, options app.SidebarUIOptions, stdout io.Writer) error {
	model := uity.NewTreeSidebarModelWithOptions(items, toUIActions(actions), uity.SidebarOptions{
		ShowNumericItems:        options.ShowNumericItems,
		Version:                 options.Version,
		CheckUpdateAvailable:    options.CheckUpdateAvailable,
		MetadataIconMode:        uity.MetadataIconMode(options.MetadataIconMode),
		AgentAttentionAnimation: options.AgentAttentionAnimation,
	})
	program := tea.NewProgram(model, tea.WithOutput(stdout), tea.WithContext(ctx))
	_, err := program.Run()
	return err
}

func toUIActions(actions app.SidebarUIActions) uity.Actions {
	return uity.Actions{
		SwitchSession:               actions.SwitchSession,
		CreateProject:               actions.CreateProject,
		CreateGitProject:            actions.CreateGitProject,
		CreateAdhoc:                 actions.CreateAdhoc,
		CreateNamedSession:          actions.CreateNamedSession,
		KillSession:                 actions.KillSession,
		TogglePinnedSession:         actions.TogglePinnedSession,
		ColorSession:                actions.ColorSession,
		ColorCategory:               actions.ColorCategory,
		SetShowNumericItems:         actions.SetShowNumericItems,
		LoadProjects:                actions.LoadProjects,
		ReloadTreeItems:             actions.ReloadTreeItems,
		CreateCategory:              actions.CreateCategory,
		RenameCategory:              actions.RenameCategory,
		CreateSpacer:                actions.CreateSpacer,
		CreateSeparator:             actions.CreateSeparator,
		MoveTreeItem:                actions.MoveTreeItem,
		DeleteTreeItem:              actions.DeleteTreeItem,
		SetCategoryCollapsed:        actions.SetCategoryCollapsed,
		SetCategorySessionsExpanded: actions.SetCategorySessionsExpanded,
		SelfUpdate: func() tea.Cmd {
			return func() tea.Msg {
				var err error
				if actions.SelfUpdate != nil {
					err = actions.SelfUpdate()
				}
				return uity.SelfUpdateFinishedMsg{Err: err}
			}
		},
	}
}
