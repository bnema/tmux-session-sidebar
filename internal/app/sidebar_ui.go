package app

import (
	"context"
	"fmt"
	"io"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/viewmodel"
)

// SidebarUIOptions configures how a sidebar UI implementation should render
// and which optional UI behaviors are enabled.
type SidebarUIOptions struct {
	ShowNumericItems        bool
	Version                 string
	CheckUpdateAvailable    func(currentVersion string) (bool, error)
	MetadataIconMode        viewmodel.MetadataIconMode
	AgentAttentionAnimation config.AgentAttentionAnimation
}

// SidebarUIActions defines the app-layer callbacks that the sidebar UI may
// invoke in response to user actions.
type SidebarUIActions struct {
	SwitchSession       func(string) bool
	CreateProject       func(viewmodel.ProjectItem, string) bool
	CreateGitProject    func(string) bool
	CreateAdhoc         func(string) bool
	CreateNamedSession  func(string, string) bool
	KillSession         func(string) bool
	TogglePinnedSession func(string) bool
	ColorSession        func(string, string) bool
	ColorCategory       func(string, string) bool
	SetShowNumericItems func(bool) bool
	// SelfUpdate returns an error so the UI can surface update failures instead
	// of collapsing them into a generic success/failure boolean.
	SelfUpdate                  func() error
	LoadProjects                func() []viewmodel.ProjectItem
	ReloadTreeItems             func() []viewmodel.TreeItem
	CreateCategory              func(string) bool
	RenameCategory              func(string, string) bool
	CreateSpacer                func() bool
	CreateSeparator             func() bool
	MoveTreeItem                func(string, int) bool
	DeleteTreeItem              func(viewmodel.TreeItem) bool
	SetCategoryCollapsed        func(string, bool) bool
	SetCategorySessionsExpanded func(string, bool) bool
}

// SidebarUIRunner renders the sidebar UI and blocks until it exits or the
// context is canceled.
type SidebarUIRunner interface {
	Run(ctx context.Context, items []viewmodel.TreeItem, actions SidebarUIActions, options SidebarUIOptions, stdout io.Writer) error
}

type missingSidebarUI struct{}

func (missingSidebarUI) Run(context.Context, []viewmodel.TreeItem, SidebarUIActions, SidebarUIOptions, io.Writer) error {
	return fmt.Errorf("sidebar UI runner unavailable")
}
