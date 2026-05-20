package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	coreruntime "github.com/bnema/tmux-session-sidebar/core/runtime"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type runtimeRouter struct {
	sidebar ports.TmuxSidebarPort
}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter(sidebar ports.TmuxSidebarPort) Router {
	return runtimeRouter{sidebar: sidebar}
}

func (r runtimeRouter) Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error {
	if routeRequiresSidebar(route.Path) && r.sidebar == nil {
		return fmt.Errorf("runtime router: sidebar port is required for %s", route.Path)
	}
	switch route.Path {
	case "sidebar/toggle":
		return toggleSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/open":
		return openSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/close":
		return closeSidebar(ctx, route.Flags, r.sidebar)
	case "ui/run":
		return runUI(ctx, route.Flags, stdout, r.sidebar)
	case "action/quick-switch":
		return quickSwitch(ctx, route.Flags, r.sidebar)
	case "action/switch":
		return switchClient(ctx, route.Flags["client"], route.Flags["session"], r.sidebar)
	case "action/toggle-numeric":
		return nil
	case "action/create-project":
		return createProject(ctx, route.Flags, stdout, r.sidebar)
	case "action/create-current-git-project":
		return createCurrentGitProject(ctx, route.Flags, r.sidebar)
	case "action/create-adhoc":
		return createAdhoc(ctx, route.Flags, r.sidebar)
	case "action/rename":
		return renameSession(ctx, route.Flags, r.sidebar)
	case "action/kill":
		return killSession(ctx, route.Flags, r.sidebar)
	case "daemon/ensure":
		return ensureRestoredAndCaptured(ctx)
	case "hook/client-attached":
		return ensureRestoredAndCaptured(ctx)
	case "hook/client-detached", "hook/client-session-changed":
		return captureLiveSidebarSessions(ctx)
	case "daemon/serve":
		return nil
	default:
		_, _ = fmt.Fprintf(stderr, "%s not implemented yet\n", route.Path)
		return fmt.Errorf("unimplemented route: %s", route.Path)
	}
}

func routeRequiresSidebar(path string) bool {
	switch path {
	case "sidebar/toggle",
		"sidebar/open",
		"sidebar/close",
		"ui/run",
		"action/quick-switch",
		"action/switch",
		"action/create-project",
		"action/create-current-git-project",
		"action/create-adhoc",
		"action/rename",
		"action/kill":
		return true
	default:
		return false
	}
}

func toggleSidebar(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	pane, err := sidebar.FindSidebarPane(ctx, flags["client"])
	if err != nil {
		return err
	}
	if pane.PaneID != "" {
		return sidebar.CloseSidebarPane(ctx, pane.PaneID)
	}
	return openSidebar(ctx, flags, sidebar)
}

func openSidebar(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	client := flags["client"]
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	uiArgs := []string{exe, "ui", "run"}
	if client != "" {
		uiArgs = append(uiArgs, "--client", client)
	}
	_, err = sidebar.OpenSidebar(ctx, client, uiArgs)
	return err
}

func closeSidebar(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	return sidebar.CloseSidebar(ctx, flags["client"])
}

func existingSidebarPane(ctx context.Context, client string, sidebar ports.TmuxSidebarPort) (string, error) {
	pane, err := sidebar.FindSidebarPane(ctx, client)
	if err != nil {
		return "", err
	}
	return pane.PaneID, nil
}

func runUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort) error {
	defer scheduleSidebarLayoutRestoreOnExit(ctx, flags, sidebar)
	items, err := loadSessionItems(ctx)
	if err != nil {
		return err
	}
	persisted, _ := loadSidebarState(ctx)
	actions := uity.Actions{
		SwitchSession: func(name string) bool {
			return handleActionError(ctx, "switch session", switchClient(ctx, flags["client"], name, sidebar))
		},
		CreateProject: func(project uity.ProjectItem) bool {
			return handleActionError(ctx, "create project session", createProject(ctx, map[string]string{"client": flags["client"], "project-path": project.Path}, stdout, sidebar))
		},
		CreateGitProject: func() bool {
			return handleActionError(ctx, "create git project session", createCurrentGitProject(ctx, map[string]string{"client": flags["client"]}, sidebar))
		},
		CreateAdhoc: func() bool {
			return handleActionError(ctx, "create ad-hoc session", createAdhoc(ctx, map[string]string{"client": flags["client"]}, sidebar))
		},
		RenameSession: func(name string) bool {
			return handleActionError(ctx, "rename session", renameSession(ctx, map[string]string{"client": flags["client"], "session": name}, sidebar))
		},
		KillSession: func(name string) bool {
			return handleActionError(ctx, "kill session", killSession(ctx, map[string]string{"client": flags["client"], "session": name, "confirmed": "yes"}, sidebar))
		},
		ReorderSession: func(name string, delta int) bool {
			items, err := loadSessionItems(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for reorder", err)
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.Name)
			}
			return handleActionError(ctx, "reorder session", saveMovedSessionOrder(ctx, names, name, delta))
		},
		SetShowNumericItems: func(show bool) bool {
			return handleActionError(ctx, "save sidebar state", saveShowNumericSessions(ctx, show))
		},
		LoadProjects: func() []uity.ProjectItem { return loadProjectItems(ctx) },
		ReloadSessions: func() []uity.SessionItem {
			items, err := loadSessionItems(ctx)
			if err != nil {
				handleActionError(ctx, "reload sessions", err)
				return nil
			}
			return items
		},
	}
	options := uity.SidebarOptions{}
	if persisted.Sidebar != nil {
		options.ShowNumericItems = persisted.Sidebar.ShowNumericSessions
	}
	program := tea.NewProgram(uity.NewSidebarModelWithOptions(items, actions, options), tea.WithOutput(stdout))
	_, err = program.Run()
	return err
}

func scheduleSidebarLayoutRestoreOnExit(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) {
	pane := strings.TrimSpace(flags["pane"])
	if pane == "" {
		pane = strings.TrimSpace(os.Getenv("TMUX_PANE"))
	}
	if err := sidebar.ScheduleSidebarRestoreOnExit(ctx, flags["client"], pane); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: schedule sidebar restore failed for client %q pane %q: %v\n", flags["client"], pane, err)
	}
}

func handleActionError(ctx context.Context, action string, err error) bool {
	if err == nil {
		return true
	}
	_, _ = tmux(ctx, "display-message", fmt.Sprintf("tmux-session-sidebar: %s failed: %v", action, err))
	return false
}

func quickSwitch(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	slot, err := strconv.Atoi(flags["slot"])
	if err != nil || slot <= 0 {
		return fmt.Errorf("invalid quick-switch slot %q", flags["slot"])
	}
	views, err := runtimeService().SessionViews(ctx)
	if err != nil {
		return err
	}
	visible := sessions.FilterVisible(views, false)
	names := sessions.ApplyOrder(sessionNames(visible), loadSessionOrder(ctx))
	ordered := make([]sessions.View, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, sessions.View{Name: name, Visible: true})
	}
	target, err := coreruntime.QuickSwitchTarget(ordered, slot)
	if err != nil {
		return nil
	}
	return switchClient(ctx, flags["client"], target, sidebar)
}

func tmux(ctx context.Context, args ...string) (string, error) {
	return runCommand(ctx, "tmux", args...)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	return commandRunner(ctx, name, args...)
}

var commandRunner = func(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
