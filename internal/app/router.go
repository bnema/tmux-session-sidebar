package app

import (
	"context"
	"errors"
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
	sidebar   ports.TmuxSidebarPort
	ipcClient ports.IPCClientPort
	ipcServer ports.IPCServerPort
}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter(sidebar ports.TmuxSidebarPort) Router {
	return runtimeRouter{sidebar: sidebar}
}

func NewRuntimeRouter(sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort, ipcServer ports.IPCServerPort) Router {
	return runtimeRouter{sidebar: sidebar, ipcClient: ipcClient, ipcServer: ipcServer}
}

func NewDaemonRouter(sidebar ports.TmuxSidebarPort, ipcServer ports.IPCServerPort) Router {
	return NewRuntimeRouter(sidebar, nil, ipcServer)
}

func (r runtimeRouter) direct() runtimeRouter {
	r.ipcClient = nil
	return r
}

func (r runtimeRouter) Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error {
	if r.ipcClient != nil && routeUsesIPC(route.Path) {
		if err := r.sendIPC(ctx, route); err != nil {
			shouldFallback := r.sidebar != nil && ipcUnavailableForBootstrap(err)
			if !shouldFallback {
				return err
			}
		} else {
			return nil
		}
	}
	if routeRequiresSidebar(route.Path) && r.sidebar == nil {
		return fmt.Errorf("runtime router: sidebar port is required for %s", route.Path)
	}
	switch route.Path {
	case "sidebar/toggle":
		return toggleSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/open":
		return openSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/close":
		return closeSidebar(ctx, r.sidebar)
	case "sidebar/refresh":
		return refreshSidebar(ctx, route.Flags["client"], r.sidebar)
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
		return ensureRestoredAndCapturedOnStartup(ctx)
	case "daemon/serve-ui":
		return serveSidebarUI(ctx, route.Flags, stdout, r.sidebar)
	case "hook/client-attached":
		return ensureRestoredAndCapturedAndRefresh(ctx)
	case "hook/client-detached":
		return captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, false)
	case "hook/client-session-changed":
		return captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, true)
	case "hook/client-resized", "hook/window-resized":
		return syncSidebarWidth(ctx, route.Flags)
	case "hook/agent-event":
		return recordAgentHookEvent(ctx, route.Flags)
	case "hooks/run":
		return runHooksCommand(ctx, route.Args, route.Flags, stdout, stderr)
	case "daemon/serve":
		return serveSidebarDaemon(ctx, r.ipcServer, r.direct())
	default:
		_, _ = fmt.Fprintf(stderr, "%s not implemented yet\n", route.Path)
		return fmt.Errorf("unimplemented route: %s", route.Path)
	}
}

func ipcUnavailableForBootstrap(err error) bool {
	return errors.Is(err, ports.ErrIPCConnectionRefused) || errors.Is(err, ports.ErrIPCSocketMissing) || errors.Is(err, ports.ErrIPCConnectionReset)
}

func routeUsesIPC(path string) bool {
	switch path {
	case "sidebar/toggle", "sidebar/open", "sidebar/close", "sidebar/refresh":
		return true
	default:
		return false
	}
}

func (r runtimeRouter) sendIPC(ctx context.Context, route Route) error {
	client := route.Flags["client"]
	var (
		resp ports.Response
		err  error
	)
	switch route.Path {
	case "sidebar/toggle":
		resp, err = r.ipcClient.Send(ctx, ports.SidebarToggleRequest(client))
	case "sidebar/open":
		resp, err = r.ipcClient.Send(ctx, ports.SidebarOpenRequest(client, route.Flags["width"]))
	case "sidebar/close":
		resp, err = r.ipcClient.Send(ctx, ports.SidebarCloseRequest(client))
	case "sidebar/refresh":
		resp, err = r.ipcClient.Send(ctx, ports.SidebarRefreshRequest(client))
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("daemon IPC %s failed: %s", route.Path, strings.TrimSpace(resp.Message))
	}
	return nil
}

func routeRequiresSidebar(path string) bool {
	switch path {
	case "sidebar/toggle",
		"sidebar/open",
		"sidebar/close",
		"sidebar/refresh",
		"daemon/serve-ui",
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
	client := strings.TrimSpace(flags["client"])
	logical, err := persistedSidebarState(ctx)
	if err != nil {
		return err
	}
	if logical.Open && strings.TrimSpace(logical.OwnerClient) == client {
		return closeSidebar(ctx, sidebar)
	}
	pane, err := sidebar.FindSidebarPane(ctx, client)
	if err != nil {
		return err
	}
	if pane.PaneID != "" {
		return closeSidebar(ctx, sidebar)
	}
	return openSidebar(ctx, flags, sidebar)
}

func openSidebar(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	return openSidebarForClient(ctx, flags["client"], flags["width"], sidebar)
}

func openSidebarForClient(ctx context.Context, client string, width string, sidebar ports.TmuxSidebarPort) error {
	if err := saveSidebarVisibility(ctx, true, client); err != nil {
		return err
	}
	singleton, err := ensureSingletonSidebarPane(ctx, sidebar)
	if err != nil {
		rollbackSidebarVisibility(ctx, client, err)
		return err
	}
	width = strings.TrimSpace(width)
	if width == "" {
		cfg := loadSidebarConfig(ctx)
		width = cfg.Width
	}
	if _, err = sidebar.AttachSingletonSidebar(ctx, client, singleton.PaneID, width); err != nil {
		rollbackSidebarVisibility(ctx, client, err)
		return err
	}
	return nil
}

func rollbackSidebarVisibility(ctx context.Context, client string, original error) {
	if err := saveSidebarVisibility(ctx, false, client); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: rollback sidebar visibility after open failure failed: %v (original error: %v)\n", err, original)
	}
}

func closeSidebar(ctx context.Context, sidebar ports.TmuxSidebarPort) error {
	singleton, err := sidebar.FindSingletonSidebar(ctx)
	if err != nil {
		return err
	}
	if err := parkVisibleSidebar(ctx, sidebar, singleton.PaneID); err != nil {
		return err
	}
	return saveSidebarVisibility(ctx, false, "")
}

func ensureSingletonSidebarPane(ctx context.Context, sidebar ports.TmuxSidebarPort) (ports.PaneRef, error) {
	exe, err := os.Executable()
	if err != nil {
		return ports.PaneRef{}, err
	}
	return sidebar.EnsureSingletonSidebar(ctx, []string{exe, "daemon", "serve-ui"})
}

func parkVisibleSidebar(ctx context.Context, sidebar ports.TmuxSidebarPort, paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return nil
	}
	return sidebar.ParkSingletonSidebar(ctx, paneID)
}

func syncSidebarWidth(ctx context.Context, flags map[string]string) error {
	paneID := strings.TrimSpace(flags["pane"])
	if paneID == "" {
		windowID := strings.TrimSpace(flags["window"])
		if windowID == "" {
			var err error
			windowID, err = windowIDForResizeHook(ctx, strings.TrimSpace(flags["client"]))
			if err != nil {
				return err
			}
		}
		if windowID == "" {
			return nil
		}
		var err error
		paneID, err = sidebarPaneForWindow(ctx, windowID)
		if err != nil {
			return err
		}
		if paneID == "" {
			return nil
		}
	}
	return resizeSidebarPaneToConfiguredWidth(ctx, paneID)
}

func resizeSidebarPaneToConfiguredWidth(ctx context.Context, paneID string) error {
	widthOutput, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-width")
	if err != nil {
		return tmuxCommandError("show @session-sidebar-width", widthOutput, err)
	}
	width := strings.TrimSpace(widthOutput)
	if width == "" {
		width = "20"
	}
	resizeOutput, err := tmux(ctx, "resize-pane", "-t", paneID, "-x", width)
	if err != nil && tmuxTargetGoneOutput(resizeOutput) {
		return nil
	}
	if err != nil {
		return tmuxCommandError("resize sidebar pane", resizeOutput, err)
	}
	return nil
}

func windowIDForResizeHook(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil
	}
	output, err := tmux(ctx, "display-message", "-p", "-t", target, "#{window_id}")
	if err != nil {
		if tmuxTargetGoneOutput(output) {
			return "", nil
		}
		return "", tmuxCommandError("resolve resize hook window", output, err)
	}
	return firstTrimmedLine(output), nil
}

func sidebarPaneForWindow(ctx context.Context, windowID string) (string, error) {
	output, err := tmux(ctx, "list-panes", "-t", windowID, "-f", "#{==:#{@session-sidebar-pane},1}", "-F", "#{pane_id}")
	if err != nil {
		if tmuxTargetGoneOutput(output) {
			return "", nil
		}
		return "", tmuxCommandError("find sidebar pane", output, err)
	}
	return firstTrimmedLine(output), nil
}

func firstTrimmedLine(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	if line, _, ok := strings.Cut(trimmed, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return trimmed
}

func tmuxCommandError(action string, output string, err error) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w (output: %s)", action, err, output)
}

func tmuxTargetGoneOutput(output string) bool {
	message := strings.ToLower(output)
	return strings.Contains(message, "no such window") ||
		strings.Contains(message, "can't find window") ||
		strings.Contains(message, "no such pane") ||
		strings.Contains(message, "can't find pane") ||
		strings.Contains(message, "no such client") ||
		strings.Contains(message, "can't find client")
}

var runSidebarUI = runUI

func serveSidebarUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort) error {
	return runSidebarUI(ctx, flags, stdout, sidebar)
}

func runUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort) error {
	defer scheduleSidebarLayoutRestoreOnExit(ctx, flags, sidebar)
	items, err := loadSessionItems(ctx)
	if err != nil {
		return err
	}
	persisted, _ := loadSidebarState(ctx)
	client := effectiveUIClient(ctx, flags)
	actions := uity.Actions{
		SwitchSession: func(name string) bool {
			return handleActionError(ctx, "switch session", switchClient(ctx, client, name, sidebar))
		},
		CreateProject: func(project uity.ProjectItem) bool {
			return handleActionError(ctx, "create project session", createProject(ctx, map[string]string{"client": client, "project-path": project.Path}, stdout, sidebar))
		},
		CreateGitProject: func() bool {
			return handleActionError(ctx, "create git project session", createCurrentGitProject(ctx, map[string]string{"client": client}, sidebar))
		},
		CreateAdhoc: func() bool {
			return handleActionError(ctx, "create ad-hoc session", createAdhoc(ctx, map[string]string{"client": client}, sidebar))
		},
		RenameSession: func(name string) bool {
			return handleActionError(ctx, "rename session", renameSession(ctx, map[string]string{"client": client, "session": name}, sidebar))
		},
		KillSession: func(name string) bool {
			return handleActionError(ctx, "kill session", killSession(ctx, map[string]string{"client": client, "session": name, "confirmed": "yes"}, sidebar))
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
			state, err := loadSidebarState(ctx)
			if err != nil {
				return handleActionError(ctx, "reorder session", err)
			}
			showNumeric := false
			if state.Sidebar != nil {
				showNumeric = state.Sidebar.ShowNumericSessions
			}
			return handleActionError(ctx, "reorder session", saveMovedVisibleSessionOrder(ctx, names, name, delta, showNumeric))
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
	options := uity.SidebarOptions{Version: version}
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

func captureLiveSidebarSessionsAndRefresh(ctx context.Context, client string, session string, sidebar ports.TmuxSidebarPort, reconcile bool) error {
	if err := captureLiveSidebarSessions(ctx); err != nil {
		return err
	}
	if reconcile && !isInternalHookSession(session) {
		if err := reconcileSidebarVisibilityForClient(ctx, client, sidebar); err != nil {
			return err
		}
	}
	refreshAllSidebarPanesBestEffort(ctx)
	return nil
}

func isInternalHookSession(session string) bool {
	return strings.HasPrefix(strings.TrimSpace(session), "__")
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
