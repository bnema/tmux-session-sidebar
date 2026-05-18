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
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

type runtimeRouter struct{}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter() Router { return runtimeRouter{} }

func (runtimeRouter) Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error {
	switch route.Path {
	case "sidebar/toggle":
		return toggleSidebar(ctx, route.Flags)
	case "sidebar/open":
		return openSidebar(ctx, route.Flags)
	case "sidebar/close":
		return closeSidebar(ctx, route.Flags)
	case "ui/run":
		return runUI(ctx, route.Flags, stdout)
	case "action/quick-switch":
		return quickSwitch(ctx, route.Flags)
	case "action/switch":
		return switchClient(ctx, route.Flags["client"], route.Flags["session"])
	case "action/toggle-numeric":
		return nil
	case "action/create-project":
		return createProject(ctx, route.Flags, stdout)
	case "action/create-current-git-project":
		return createCurrentGitProject(ctx, route.Flags)
	case "action/create-adhoc":
		return createAdhoc(ctx, route.Flags)
	case "action/rename":
		return renameSession(ctx, route.Flags)
	case "action/kill":
		return killSession(ctx, route.Flags)
	case "daemon/serve", "daemon/ensure", "hook/client-attached", "hook/client-detached", "hook/client-session-changed":
		return nil
	default:
		_, _ = fmt.Fprintf(stderr, "%s not implemented yet\n", route.Path)
		return fmt.Errorf("unimplemented route: %s", route.Path)
	}
}

func toggleSidebar(ctx context.Context, flags map[string]string) error {
	pane, err := existingSidebarPane(ctx, flags["client"])
	if err != nil {
		return err
	}
	if pane != "" {
		_, err := tmux(ctx, "kill-pane", "-t", pane)
		return err
	}
	return openSidebar(ctx, flags)
}

func openSidebar(ctx context.Context, flags map[string]string) error {
	client := flags["client"]
	width, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-width")
	if err != nil || strings.TrimSpace(width) == "" {
		width = "20"
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	windowTarget := client
	if windowTarget == "" {
		windowTarget = "#{window_id}"
	}
	windowID, err := tmux(ctx, "display-message", "-p", "-t", windowTarget, "#{window_id}")
	if err != nil {
		return err
	}
	currentPath, err := tmux(ctx, "display-message", "-p", "-t", windowTarget, "#{pane_current_path}")
	if err != nil {
		currentPath = ""
	}
	args := []string{"split-window", "-P", "-F", "#{pane_id}", "-t", strings.TrimSpace(windowID), "-hbf", "-l", strings.TrimSpace(width)}
	if strings.TrimSpace(currentPath) != "" {
		args = append(args, "-c", strings.TrimSpace(currentPath))
	}
	uiArgs := []string{exe, "ui", "run"}
	if client != "" {
		uiArgs = append(uiArgs, "--client", client)
	}
	args = append(args, uiArgs...)
	pane, err := tmux(ctx, args...)
	if err != nil {
		return err
	}
	_, err = tmux(ctx, "set-option", "-p", "-t", strings.TrimSpace(pane), "@session-sidebar-pane", "1")
	return err
}

func closeSidebar(ctx context.Context, flags map[string]string) error {
	pane, err := existingSidebarPane(ctx, flags["client"])
	if err != nil || pane == "" {
		return err
	}
	_, err = tmux(ctx, "kill-pane", "-t", pane)
	return err
}

func existingSidebarPane(ctx context.Context, client string) (string, error) {
	windowTarget := client
	if windowTarget == "" {
		windowTarget = "#{window_id}"
	}
	window, err := tmux(ctx, "display-message", "-p", "-t", windowTarget, "#{window_id}")
	if err != nil {
		return "", err
	}
	out, err := tmux(ctx, "list-panes", "-t", strings.TrimSpace(window), "-F", "#{pane_id}\t#{@session-sidebar-pane}")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0], nil
		}
	}
	return "", nil
}

func runUI(ctx context.Context, flags map[string]string, stdout io.Writer) error {
	items, err := loadSessionItems(ctx)
	if err != nil {
		return err
	}
	actions := uity.Actions{
		SwitchSession: func(name string) bool {
			return handleActionError(ctx, "switch session", switchClient(ctx, flags["client"], name))
		},
		CreateProject: func(project uity.ProjectItem) bool {
			return handleActionError(ctx, "create project session", createProject(ctx, map[string]string{"client": flags["client"], "project-path": project.Path}, stdout))
		},
		CreateGitProject: func() bool {
			return handleActionError(ctx, "create git project session", createCurrentGitProject(ctx, map[string]string{"client": flags["client"]}))
		},
		CreateAdhoc: func() bool {
			return handleActionError(ctx, "create ad-hoc session", createAdhoc(ctx, map[string]string{"client": flags["client"]}))
		},
		RenameSession: func(name string) bool {
			return handleActionError(ctx, "rename session", renameSession(ctx, map[string]string{"client": flags["client"], "session": name}))
		},
		KillSession: func(name string) bool {
			return handleActionError(ctx, "kill session", killSession(ctx, map[string]string{"client": flags["client"], "session": name, "confirmed": "yes"}))
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
	program := tea.NewProgram(uity.NewSidebarModel(items, actions), tea.WithOutput(stdout))
	_, err = program.Run()
	return err
}

func handleActionError(ctx context.Context, action string, err error) bool {
	if err == nil {
		return true
	}
	_, _ = tmux(ctx, "display-message", fmt.Sprintf("tmux-session-sidebar: %s failed: %v", action, err))
	return false
}

func quickSwitch(ctx context.Context, flags map[string]string) error {
	slot, err := strconv.Atoi(flags["slot"])
	if err != nil || slot <= 0 {
		return fmt.Errorf("invalid quick-switch slot %q", flags["slot"])
	}
	out, err := tmux(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return err
	}
	names := visibleSessionNames(out)
	if slot > len(names) {
		return nil
	}
	args := []string{"switch-client"}
	if flags["client"] != "" {
		args = append(args, "-c", flags["client"])
	}
	args = append(args, "-t", names[slot-1])
	_, err = tmux(ctx, args...)
	return err
}

func visibleSessionNames(out string) []string {
	var names []string
	for name := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		name = strings.TrimSpace(name)
		if name != "" && !sessions.IsNumericName(name) && !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	return names
}

func tmux(ctx context.Context, args ...string) (string, error) {
	return runCommand(ctx, "tmux", args...)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
