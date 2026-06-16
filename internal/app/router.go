package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	coreruntime "github.com/bnema/tmux-session-sidebar/internal/core/runtime"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/internal/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/viewmodel"
)

type runtimeRouter struct {
	sidebar        ports.SidebarPort
	ipcClient      ports.IPCClientPort
	ipcServer      ports.IPCServerPort
	daemonLauncher ports.DaemonLauncherPort
}

type RuntimeRouterSnapshot struct {
	Sidebar        ports.SidebarPort
	IPCClient      ports.IPCClientPort
	IPCServer      ports.IPCServerPort
	DaemonLauncher ports.DaemonLauncherPort
}

func InspectRuntimeRouter(router Router) (RuntimeRouterSnapshot, bool) {
	r, ok := router.(runtimeRouter)
	if !ok {
		return RuntimeRouterSnapshot{}, false
	}
	return RuntimeRouterSnapshot{Sidebar: r.sidebar, IPCClient: r.ipcClient, IPCServer: r.ipcServer, DaemonLauncher: r.daemonLauncher}, true
}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter(sidebar ports.SidebarPort) Router {
	return runtimeRouter{sidebar: sidebar}
}

func NewRuntimeRouter(sidebar ports.SidebarPort, ipcClient ports.IPCClientPort, ipcServer ports.IPCServerPort) Router {
	return NewRuntimeRouterWithDaemon(sidebar, ipcClient, ipcServer, nil)
}

func NewRuntimeRouterWithDaemon(sidebar ports.SidebarPort, ipcClient ports.IPCClientPort, ipcServer ports.IPCServerPort, daemonLauncher ports.DaemonLauncherPort) Router {
	return runtimeRouter{sidebar: sidebar, ipcClient: ipcClient, ipcServer: ipcServer, daemonLauncher: daemonLauncher}
}

func NewDaemonRouter(sidebar ports.SidebarPort, ipcServer ports.IPCServerPort) Router {
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
	case "daemon/bootstrap":
		return bootstrapSidebarDaemon(ctx, stderr, r.ipcServer, r.direct())
	case "daemon/serve-ui":
		return serveSidebarUI(ctx, route.Flags, stdout, r.sidebar, r.ipcClient)
	case "hook/client-attached":
		return r.withMetadataReconcile(ctx, ensureRestoredAndCapturedAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar))
	case "hook/client-detached":
		return r.withMetadataReconcile(ctx, captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, false))
	case "hook/client-session-changed":
		return r.withMetadataReconcile(ctx, captureLiveSidebarSessionsAndRefresh(ctx, route.Flags["client"], route.Flags["session"], r.sidebar, true))
	case "hook/client-resized", "hook/window-resized":
		return syncSidebarWidth(ctx, route.Path, route.Flags, r.sidebar)
	case "hook/window-layout-changed":
		return captureSidebarWidthBaseline(ctx, route.Flags, r.sidebar)
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
	return errors.Is(err, ports.ErrIPCConnectionRefused) || errors.Is(err, ports.ErrIPCSocketMissing) || errors.Is(err, ports.ErrIPCConnectionReset) || errors.Is(err, ports.ErrIPCStaleScope)
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
		message := strings.TrimSpace(resp.Message)
		if resp.ErrorCode == ports.IPCErrorStaleScope {
			return fmt.Errorf("%w: daemon IPC %s failed: %s", ports.ErrIPCStaleScope, route.Path, message)
		}
		return fmt.Errorf("daemon IPC %s failed: %s", route.Path, message)
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

func toggleSidebar(ctx context.Context, flags map[string]string, sidebar ports.SidebarPort) error {
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

func openSidebar(ctx context.Context, flags map[string]string, sidebar ports.SidebarPort) error {
	return openSidebarForClient(ctx, flags["client"], flags["attach-target"], flags["width"], sidebar)
}

func openSidebarForClient(ctx context.Context, client string, attachTarget string, width string, sidebar ports.SidebarPort) error {
	return openSidebarForClientWith(ctx, client, attachTarget, width, sidebar, sidebar.AttachSingletonSidebar)
}

func openSidebarForClientWithoutFocus(ctx context.Context, client string, attachTarget string, width string, sidebar ports.SidebarPort) error {
	attach := sidebar.AttachSingletonSidebar
	if follower, ok := sidebar.(ports.SidebarFollowPort); ok {
		attach = follower.AttachSingletonSidebarWithoutFocus
	}
	return openSidebarForClientWith(ctx, client, attachTarget, width, sidebar, attach)
}

func openSidebarForClientWith(ctx context.Context, client string, attachTarget string, width string, sidebar ports.SidebarPort, attach func(context.Context, string, string, string) (ports.PaneRef, error)) error {
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
	_, err = attach(ctx, attachTarget, singleton.PaneID, width)
	if err != nil {
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

func closeSidebar(ctx context.Context, sidebar ports.SidebarPort) error {
	singleton, err := sidebar.FindSingletonSidebar(ctx)
	if err != nil {
		return err
	}
	if err := parkVisibleSidebar(ctx, sidebar, singleton.PaneID); err != nil {
		return err
	}
	return saveSidebarVisibility(ctx, false, "")
}

func ensureSingletonSidebarPane(ctx context.Context, sidebar ports.SidebarPort) (ports.PaneRef, error) {
	exe, err := os.Executable()
	if err != nil {
		return ports.PaneRef{}, err
	}
	return sidebar.EnsureSingletonSidebar(ctx, []string{exe, "daemon", "serve-ui"})
}

func parkVisibleSidebar(ctx context.Context, sidebar ports.SidebarPort, paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return nil
	}
	return sidebar.ParkSingletonSidebar(ctx, paneID)
}

func syncSidebarWidth(ctx context.Context, _ string, flags map[string]string, sidebar ports.SidebarPort) error {
	if sidebar == nil {
		return nil
	}
	ref, err := findSidebarForHook(ctx, sidebar, flags)
	if err != nil {
		if isIgnoredResizeHookError(err) {
			return nil
		}
		return err
	}
	if ref.PaneID == "" {
		return nil
	}
	width, err := configuredSidebarWidth(ctx)
	if err != nil {
		return err
	}
	if syncer, ok := sidebar.(ports.SidebarResizePort); ok {
		return syncer.SyncAttachedSidebarWidth(ctx, ref.WindowID, ref.PaneID, width)
	}
	return resizeSidebarPaneToWidth(ctx, ref.PaneID, width)
}

func captureSidebarWidthBaseline(ctx context.Context, flags map[string]string, sidebar ports.SidebarPort) error {
	if sidebar == nil {
		return nil
	}
	capturer, ok := sidebar.(ports.SidebarResizePort)
	if !ok {
		return nil
	}
	ref, err := findSidebarForHook(ctx, sidebar, flags)
	if err != nil {
		if isIgnoredResizeHookError(err) {
			return nil
		}
		return err
	}
	if ref.PaneID == "" {
		return nil
	}
	width, err := configuredSidebarWidth(ctx)
	if err != nil {
		return err
	}
	return capturer.CaptureAttachedSidebarWidthBaseline(ctx, ref.WindowID, ref.PaneID, width)
}

func findSidebarForHook(ctx context.Context, sidebar ports.SidebarPort, flags map[string]string) (ports.PaneRef, error) {
	target := firstNonEmpty(strings.TrimSpace(flags["pane"]), strings.TrimSpace(flags["window"]), strings.TrimSpace(flags["client"]))
	if target == "" {
		return ports.PaneRef{}, nil
	}
	return sidebar.FindSidebarPane(ctx, target)
}

func configuredSidebarWidth(ctx context.Context) (string, error) {
	widthOutput, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-width")
	if err != nil {
		return "", tmuxCommandError("show @session-sidebar-width", widthOutput, err)
	}
	width := strings.TrimSpace(widthOutput)
	if width == "" {
		width = "30"
	}
	return width, nil
}

func resizeSidebarPaneToWidth(ctx context.Context, paneID string, width string) error {
	resizeOutput, err := tmux(ctx, "resize-pane", "-t", paneID, "-x", width)
	if err != nil && tmuxTargetGoneOutput(resizeOutput) {
		return nil
	}
	if err != nil {
		return tmuxCommandError("resize sidebar pane", resizeOutput, err)
	}
	return nil
}

func isIgnoredResizeHookError(err error) bool {
	return errors.Is(err, ports.ErrMultiplexerTargetGone)
}

// tmuxCommandError wraps a tmux command error with the action context and any
// captured stderr output. Part of the transitional tmux command seam.
func tmuxCommandError(action string, output string, err error) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w (output: %s)", action, err, output)
}

// tmuxTargetGoneOutput checks whether a tmux command output indicates a
// disappeared target (window, pane, or client), mapping to ErrMultiplexerTargetGone.
// Part of the transitional tmux command seam.
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

func serveSidebarUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.SidebarPort, ipcClient ports.IPCClientPort) error {
	return runSidebarUI(ctx, flags, stdout, sidebar, ipcClient)
}

func runUI(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.SidebarPort, ipcClient ports.IPCClientPort) error {
	defer scheduleSidebarLayoutRestoreOnExit(ctx, flags, sidebar)
	cfg := loadSidebarConfig(ctx)
	items, err := loadSidebarTreeItemsWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	persisted, _ := loadSidebarState(ctx)
	actions := buildSidebarActions(ctx, flags, stdout, sidebar, ipcClient)
	options := SidebarUIOptions{Version: version, CheckUpdateAvailable: newUpdateAvailableCheck(ctx, runtimeReleaseChecker()), AgentAttentionAnimation: cfg.AgentAttentionAnimation, Appearance: cfg.ColorSchemeAppearance}
	if persisted.Sidebar != nil {
		options.ShowNumericItems = persisted.Sidebar.ShowNumericSessions
	}
	return runtimeSidebarUI().Run(ctx, items, actions, options, stdout)
}

func buildSidebarActions(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.SidebarPort, ipcClient ports.IPCClientPort) SidebarUIActions {
	currentClient := func() string { return effectiveUIClient(ctx, flags) }
	return SidebarUIActions{
		SwitchSession: func(name string) bool {
			return handleActionError(ctx, "switch session", switchClient(ctx, currentClient(), name, sidebar))
		},
		CreateProject: func(project viewmodel.ProjectItem, categoryID string) bool {
			return handleMetadataAction(ctx, ipcClient, "create project session", createProject(ctx, map[string]string{"client": currentClient(), "project-path": project.Path, "category-id": categoryID}, stdout, sidebar))
		},
		CreateGitProject: func(categoryID string) bool {
			return handleMetadataAction(ctx, ipcClient, "create git project session", createCurrentGitProject(ctx, map[string]string{"client": currentClient(), "category-id": categoryID}, sidebar))
		},
		CreateAdhoc: func(categoryID string) bool {
			return handleMetadataAction(ctx, ipcClient, "create ad-hoc session", createAdhoc(ctx, map[string]string{"client": currentClient(), "category-id": categoryID}, sidebar))
		},
		CreateNamedSession: func(name string, categoryID string) bool {
			return handleMetadataAction(ctx, ipcClient, "create named session", createAdhoc(ctx, map[string]string{"client": currentClient(), "name": name, "category-id": categoryID}, sidebar))
		},
		KillSession: func(name string) bool {
			return handleMetadataAction(ctx, ipcClient, "kill session", killSession(ctx, map[string]string{"client": currentClient(), "session": name, "confirmed": "yes"}, sidebar))
		},
		TogglePinnedSession: func(name string) bool {
			names, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for pin", err)
			}
			return handleActionError(ctx, "toggle pinned session", saveToggledPinnedSession(ctx, names, name))
		},
		ColorSession: func(name string, color string) bool {
			names, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for session color", err)
			}
			return handleActionError(ctx, "color session", saveSessionColor(ctx, names, name, color))
		},
		ColorCategory: func(categoryID string, color string) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for category color", err)
			}
			return handleActionError(ctx, "color category", saveSidebarCategoryColor(ctx, categoryID, color, live))
		},
		SetShowNumericItems: func(show bool) bool {
			return handleActionError(ctx, "save sidebar state", saveShowNumericSessions(ctx, show))
		},
		SelfUpdate: func() error {
			return runSelfUpdateBackground()
		},
		LoadProjects: func() []viewmodel.ProjectItem { return loadProjectItems(ctx) },
		ReloadTree: func() *SidebarReloadResult {
			cfg := loadSidebarConfig(ctx)
			items, err := loadSidebarTreeItemsWithConfig(ctx, cfg)
			if err != nil {
				handleActionError(ctx, "reload sidebar tree", err)
				return nil
			}
			return &SidebarReloadResult{Items: items, Appearance: cfg.ColorSchemeAppearance}
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
		DeleteTreeItem: func(item viewmodel.TreeItem) bool {
			if item.Kind == viewmodel.TreeRowSession {
				return handleMetadataAction(ctx, ipcClient, "delete session", killSession(ctx, map[string]string{"client": currentClient(), "session": item.Session.Name, "confirmed": "yes"}, sidebar))
			}
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for layout delete", err)
			}
			selection := sidebarlayoutSelectionForItem(item.ID)
			return handleActionError(ctx, "delete sidebar item", saveDeletedSidebarLayoutItem(ctx, selection, live))
		},
		SetCategoryCollapsed: func(categoryID string, collapsed bool) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for category collapse", err)
			}
			return handleActionError(ctx, "set category collapse", saveSidebarCategoryCollapsed(ctx, categoryID, collapsed, live))
		},
		SetCategorySessionsExpanded: func(categoryID string, expanded bool) bool {
			live, err := currentLiveSessionNames(ctx)
			if err != nil {
				return handleActionError(ctx, "load sessions for category overflow", err)
			}
			return handleActionError(ctx, "set category overflow", saveSidebarCategorySessionsExpanded(ctx, categoryID, expanded, live))
		},
	}
}

func scheduleSidebarLayoutRestoreOnExit(ctx context.Context, flags map[string]string, sidebar ports.SidebarPort) {
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

func captureLiveSidebarSessionsAndRefresh(ctx context.Context, client string, session string, sidebar ports.SidebarPort, reconcile bool) error {
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

func quickSwitch(ctx context.Context, flags map[string]string, sidebar ports.SidebarPort) error {
	slot, err := strconv.Atoi(flags["slot"])
	if err != nil || slot <= 0 {
		return fmt.Errorf("invalid quick-switch slot %q", flags["slot"])
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		return err
	}
	views, err := runtimeService().SessionViews(ctx)
	if err != nil {
		return err
	}
	layoutVisible := sessions.FilterVisible(views, true)
	fallbackVisible := sessions.FilterVisible(views, persistedShowNumeric(state))
	target, err := contextualQuickSwitchTarget(ctx, state, layoutVisible, fallbackVisible, slot)
	if err != nil {
		return nil
	}
	return switchClient(ctx, flags["client"], target, sidebar)
}

func contextualQuickSwitchTarget(ctx context.Context, state ports.PersistedState, layoutVisible []sessions.View, fallbackVisible []sessions.View, slot int) (string, error) {
	current, err := tmux(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		current = ""
	}
	layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), sessionNames(layoutVisible), state.SessionOrder)
	active := sidebarlayout.ActiveCategoryID(layout, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSession, Session: strings.TrimSpace(current)})
	if active != "" {
		for _, row := range sidebarlayout.Flatten(layout, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSession, Session: strings.TrimSpace(current)}, persistedShowNumeric(state)) {
			if row.Kind == sidebarlayout.RowKindSession && row.CategoryID == active && row.Slot == slot {
				return row.Session, nil
			}
		}
		return "", fmt.Errorf("slot %d has no visible session", slot)
	}
	names := sessions.ApplyOrder(sessionNames(fallbackVisible), state.SessionOrder)
	ordered := make([]sessions.View, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, sessions.View{Name: name, Visible: true})
	}
	return coreruntime.QuickSwitchTarget(ordered, slot)
}

// tmux is the transitional tmux command seam. It shells out raw tmux CLI
// commands for operations not yet migrated behind the generic port interfaces.
//
// All callers of this function are explicitly tmux-specific and are NOT
// part of the generic multiplexer abstraction. Future adapters (Zellij,
// etc.) must model equivalent behaviour through the appropriate port
// (SidebarPort, ControlPort, QueryPort) instead.
func tmux(ctx context.Context, args ...string) (string, error) {
	return runCommand(ctx, "tmux", args...)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	return commandRunner(ctx, name, args...)
}

// commandRunner is the transitional tmux command seam executor. It routes the
// "tmux" name through the Multiplexer runtime port and rejects all other
// command names — those must be modeled behind explicit runtime ports.
var commandRunner = func(ctx context.Context, name string, args ...string) (string, error) {
	if name != "tmux" {
		return "", fmt.Errorf("unsupported runtime command %q", name)
	}
	result, err := runtimeMultiplexer().Run(ctx, args)
	if err != nil {
		if strings.TrimSpace(result.Stderr) != "" {
			return result.Stderr, err
		}
		return result.Stdout, err
	}
	return result.Stdout, nil
}
