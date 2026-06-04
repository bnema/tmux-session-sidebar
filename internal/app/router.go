package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/adapters/githubrelease"
	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	coreruntime "github.com/bnema/tmux-session-sidebar/core/runtime"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type runtimeRouter struct {
	sidebar        ports.TmuxSidebarPort
	ipcClient      ports.IPCClientPort
	ipcServer      ports.IPCServerPort
	daemonLauncher ports.DaemonLauncherPort
}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter(sidebar ports.TmuxSidebarPort) Router {
	return runtimeRouter{sidebar: sidebar}
}

func NewRuntimeRouter(sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort, ipcServer ports.IPCServerPort) Router {
	return NewRuntimeRouterWithDaemon(sidebar, ipcClient, ipcServer, nil)
}

func NewRuntimeRouterWithDaemon(sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort, ipcServer ports.IPCServerPort, daemonLauncher ports.DaemonLauncherPort) Router {
	return runtimeRouter{sidebar: sidebar, ipcClient: ipcClient, ipcServer: ipcServer, daemonLauncher: daemonLauncher}
}

func NewDaemonRouter(sidebar ports.TmuxSidebarPort, ipcServer ports.IPCServerPort) Router {
	return NewRuntimeRouterWithDaemon(sidebar, nil, ipcServer, nil)
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
			r.ensureDaemonStartedBestEffort(ctx, route, stderr)
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
		return r.withMetadataReconcile(ctx, createProject(ctx, route.Flags, stdout, r.sidebar))
	case "action/create-current-git-project":
		return r.withMetadataReconcile(ctx, createCurrentGitProject(ctx, route.Flags, r.sidebar))
	case "action/create-adhoc":
		return r.withMetadataReconcile(ctx, createAdhoc(ctx, route.Flags, r.sidebar))
	case "action/rename":
		return r.withMetadataReconcile(ctx, renameSession(ctx, route.Flags, r.sidebar))
	case "action/kill":
		return r.withMetadataReconcile(ctx, killSession(ctx, route.Flags, r.sidebar))
	case "resurrect/post-save-layout":
		if len(route.Args) < 1 {
			return fmt.Errorf("resurrect post-save-layout: missing save file")
		}
		return resurrectPostSaveLayout(ctx, route.Args[0])
	case "daemon/ensure":
		return ensureRestoredAndCapturedOnStartup(ctx)
	case "daemon/serve-ui":
		return serveSidebarUI(ctx, route.Flags, stdout, r.sidebar, r.ipcClient)
	case "hook/client-attached":
		return r.withMetadataReconcile(ctx, ensureRestoredAndCapturedAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar))
	case "hook/client-detached":
		return r.withMetadataReconcile(ctx, captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, false))
	case "hook/client-session-changed":
		return r.withMetadataReconcile(ctx, captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, true))
	case "hook/client-resized", "hook/window-resized":
		return syncSidebarWidth(ctx, route.Flags)
	case "hook/agent-event":
		return recordAgentHookEvent(ctx, route.Flags)
	case "hooks/run":
		return runHooksCommand(ctx, route.Args, route.Flags, stdout, stderr)
	case "runtime/self-update":
		return runSelfUpdate(ctx, stdout, stderr)
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

func (r runtimeRouter) ensureDaemonStartedBestEffort(ctx context.Context, route Route, stderr io.Writer) {
	if r.daemonLauncher == nil {
		return
	}
	if err := r.daemonLauncher.EnsureStarted(ctx); err != nil && !errors.Is(err, context.Canceled) {
		if stderr == nil {
			stderr = io.Discard
		}
		_, _ = fmt.Fprintf(stderr, "tmux-session-sidebar: daemon ensure failed before direct fallback for %s: %v\n", route.Path, err)
	}
}

func (r runtimeRouter) withMetadataReconcile(ctx context.Context, err error) error {
	if err != nil {
		return err
	}
	if r.ipcClient == nil {
		return nil
	}
	notifyMetadataReconcile(ctx, r.ipcClient)
	return nil
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
	pane, err := sidebar.FindSidebarPane(ctx, client)
	if err != nil {
		return err
	}
	if logical.Open && strings.TrimSpace(logical.OwnerClient) == client && pane.PaneID != "" {
		return closeSidebar(ctx, sidebar)
	}
	if !logical.Open && pane.PaneID != "" {
		return closeSidebar(ctx, sidebar)
	}
	openFlags := flags
	if pane.WindowID != "" {
		openFlags = cloneStringMap(flags)
		openFlags["attach-target"] = pane.WindowID
	}
	return openSidebar(ctx, openFlags, sidebar)
}

func cloneStringMap(values map[string]string) map[string]string {
	return maps.Clone(values)
}

func openSidebar(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	return openSidebarForClient(ctx, flags["client"], flags["attach-target"], flags["width"], sidebar)
}

func openSidebarForClient(ctx context.Context, client string, attachTarget string, width string, sidebar ports.TmuxSidebarPort) error {
	return openSidebarForClientWith(ctx, client, attachTarget, width, sidebar, sidebar.AttachSingletonSidebar)
}

func openSidebarForClientWithoutFocus(ctx context.Context, client string, attachTarget string, width string, sidebar ports.TmuxSidebarPort) error {
	attach := sidebar.AttachSingletonSidebar
	if follower, ok := sidebar.(ports.TmuxSidebarFollowPort); ok {
		attach = follower.AttachSingletonSidebarWithoutFocus
	}
	return openSidebarForClientWith(ctx, client, attachTarget, width, sidebar, attach)
}

func openSidebarForClientWith(ctx context.Context, client string, attachTarget string, width string, sidebar ports.TmuxSidebarPort, attach func(context.Context, string, string, string) (ports.PaneRef, error)) error {
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
	if strings.TrimSpace(attachTarget) == "" {
		attachTarget = client
	}
	if _, err = attach(ctx, attachTarget, singleton.PaneID, width); err != nil {
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
		width = "30"
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

func serveSidebarUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort) error {
	return runSidebarUI(ctx, flags, stdout, sidebar, ipcClient)
}

func runUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort) error {
	defer scheduleSidebarLayoutRestoreOnExit(ctx, flags, sidebar)
	cfg := loadSidebarConfig(ctx)
	items, err := loadSidebarTreeItemsWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	persisted, _ := loadSidebarState(ctx)
	actions := buildSidebarActions(ctx, flags, stdout, sidebar, ipcClient)
	options := uity.SidebarOptions{Version: version, CheckUpdateAvailable: newUpdateAvailableCheck(ctx, githubrelease.Client{}), AgentAttentionAnimation: cfg.AgentAttentionAnimation}
	if persisted.Sidebar != nil {
		options.ShowNumericItems = persisted.Sidebar.ShowNumericSessions
	}
	program := tea.NewProgram(uity.NewTreeSidebarModelWithOptions(toUITreeItems(items), actions, options), tea.WithOutput(stdout))
	_, err = program.Run()
	return err
}

func buildSidebarActions(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.TmuxSidebarPort, ipcClient ports.IPCClientPort) uity.Actions {
	currentClient := func() string { return effectiveUIClient(ctx, flags) }
	return uity.Actions{
		SwitchSession: func(name string) bool {
			return handleActionError(ctx, "switch session", switchClient(ctx, currentClient(), name, sidebar))
		},
		CreateProject: func(project uity.ProjectItem) bool {
			return handleMetadataAction(ctx, ipcClient, "create project session", createProject(ctx, map[string]string{"client": currentClient(), "project-path": project.Path}, stdout, sidebar))
		},
		CreateGitProject: func() bool {
			return handleMetadataAction(ctx, ipcClient, "create git project session", createCurrentGitProject(ctx, map[string]string{"client": currentClient()}, sidebar))
		},
		CreateAdhoc: func() bool {
			return handleMetadataAction(ctx, ipcClient, "create ad-hoc session", createAdhoc(ctx, map[string]string{"client": currentClient()}, sidebar))
		},
		CreateNamedSession: func(name string) bool {
			return handleMetadataAction(ctx, ipcClient, "create named session", createAdhoc(ctx, map[string]string{"client": currentClient(), "name": name}, sidebar))
		},
		RenameSession: func(name string) bool {
			return handleMetadataAction(ctx, ipcClient, "rename session", renameSession(ctx, map[string]string{"client": currentClient(), "session": name}, sidebar))
		},
		KillSession: func(name string) bool {
			return handleMetadataAction(ctx, ipcClient, "kill session", killSession(ctx, map[string]string{"client": currentClient(), "session": name, "confirmed": "yes"}, sidebar))
		},
		TogglePinnedSession: func(name string) bool {
			items, err := loadSessionItems(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for pin", err)
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.Name)
			}
			return handleActionError(ctx, "toggle pinned session", saveToggledPinnedSession(ctx, names, name))
		},
		PinSessionWithColor: func(name string, color string) bool {
			items, err := loadSessionItems(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for pin color", err)
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.Name)
			}
			return handleActionError(ctx, "pin session color", savePinnedSessionColor(ctx, names, name, color))
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
		SelfUpdate: func() tea.Cmd {
			return func() tea.Msg {
				return uity.SelfUpdateFinishedMsg{Err: runSelfUpdateBackground()}
			}
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
		ReloadTreeItems: func() []uity.TreeItem {
			items, err := loadSidebarTreeItemsWithConfig(ctx, loadSidebarConfig(ctx))
			if err != nil {
				handleActionError(ctx, "reload sidebar tree", err)
				return nil
			}
			return toUITreeItems(items)
		},
		CreateCategory: func(name string) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for category create", err)
			}
			return handleActionError(ctx, "create sidebar category", saveNewSidebarCategory(ctx, name, live))
		},
		RenameCategory: func(categoryID string, name string) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for category rename", err)
			}
			return handleActionError(ctx, "rename sidebar category", saveRenamedSidebarCategory(ctx, categoryID, name, live))
		},
		CreateSpacer: func() bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for spacer create", err)
			}
			return handleActionError(ctx, "create sidebar spacer", saveNewSidebarSpacer(ctx, live))
		},
		CreateSeparator: func() bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for separator create", err)
			}
			return handleActionError(ctx, "create sidebar separator", saveNewSidebarSeparator(ctx, live))
		},
		MoveTreeItem: func(itemID string, delta int) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for layout move", err)
			}
			selection := sidebarlayoutSelectionForItem(itemID)
			return handleActionError(ctx, "move sidebar item", saveMovedSidebarLayoutItem(ctx, selection, delta, live))
		},
	}
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

func handleMetadataAction(ctx context.Context, ipcClient ports.IPCClientPort, action string, err error) bool {
	if !handleActionError(ctx, action, err) {
		return false
	}
	notifyMetadataReconcile(ctx, ipcClient)
	return true
}

func notifyMetadataReconcile(ctx context.Context, ipcClient ports.IPCClientPort) {
	if ipcClient == nil {
		return
	}
	reconcileCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, _ = ipcClient.Send(reconcileCtx, ports.MetadataReconcileRequest())
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
	session = strings.TrimSpace(session)
	return strings.HasPrefix(session, "__") || session == "tmux-session-sidebar"
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
	target, err := contextualQuickSwitchTarget(ctx, visible, slot)
	if err != nil {
		return nil
	}
	return switchClient(ctx, flags["client"], target, sidebar)
}

func contextualQuickSwitchTarget(ctx context.Context, visible []sessions.View, slot int) (string, error) {
	state, err := loadSidebarState(ctx)
	if err != nil {
		return "", err
	}
	current, _ := tmux(ctx, "display-message", "-p", "#{session_name}")
	layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), sessionNames(visible), state.SessionOrder)
	active := sidebarlayout.ActiveCategoryID(layout, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSession, Session: strings.TrimSpace(current)})
	for _, item := range layout.Items {
		if item.Kind != sidebarlayout.ItemKindCategory || item.Category.ID != active {
			continue
		}
		for name, itemSlot := range sidebarlayout.SlotMap(item.Category.Sessions, persistedShowNumeric(state)) {
			if itemSlot == slot {
				return name, nil
			}
		}
		return "", fmt.Errorf("slot %d has no session", slot)
	}
	names := sessions.ApplyOrder(sessionNames(visible), state.SessionOrder)
	ordered := make([]sessions.View, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, sessions.View{Name: name, Visible: true})
	}
	return coreruntime.QuickSwitchTarget(ordered, slot)
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
