package app

import (
	"context"
	"fmt"
	"io"
)

func (r runtimeRouter) handleIPCRoute(ctx context.Context, route Route, stderr io.Writer) (bool, error) {
	if r.ipcClient == nil || !routeUsesIPC(route.Path) {
		return false, nil
	}
	if err := r.sendIPC(ctx, route); err != nil {
		shouldFallback := r.sidebar != nil && ipcUnavailableForBootstrap(err)
		if !shouldFallback {
			return true, err
		}
		r.ensureDaemonStartedBestEffort(ctx, route, stderr)
		return false, nil
	}
	return true, nil
}

func isSidebarRoute(path string) bool {
	switch path {
	case "sidebar/toggle", "sidebar/open", "sidebar/close", "sidebar/refresh":
		return true
	default:
		return false
	}
}

func (r runtimeRouter) handleSidebarRoute(ctx context.Context, route Route) error {
	switch route.Path {
	case "sidebar/toggle":
		return toggleSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/open":
		return openSidebar(ctx, route.Flags, r.sidebar)
	case "sidebar/close":
		return closeSidebar(ctx, r.sidebar)
	case "sidebar/refresh":
		return refreshSidebar(ctx, route.Flags["client"], r.sidebar)
	default:
		return errUnhandledRoute(route.Path)
	}
}

func isActionRoute(path string) bool {
	switch path {
	case "action/quick-switch", "action/switch", "action/toggle-numeric", "action/create-project", "action/create-current-git-project", "action/create-adhoc", "action/rename", "action/kill":
		return true
	default:
		return false
	}
}

func (r runtimeRouter) handleActionRoute(ctx context.Context, route Route, stdout io.Writer) error {
	switch route.Path {
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
	default:
		return errUnhandledRoute(route.Path)
	}
}

func isHookRoute(path string) bool {
	switch path {
	case "hook/client-attached", "hook/client-detached", "hook/client-session-changed", "hook/client-resized", "hook/window-resized", "hook/window-layout-changed", "hook/agent-event":
		return true
	default:
		return false
	}
}

func (r runtimeRouter) handleHookRoute(ctx context.Context, route Route) error {
	switch route.Path {
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
	default:
		return errUnhandledRoute(route.Path)
	}
}

func (r runtimeRouter) handleRuntimeRoute(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error {
	switch route.Path {
	case "resurrect/post-save-layout":
		if len(route.Args) < 1 {
			return fmt.Errorf("resurrect post-save-layout: missing save file")
		}
		return resurrectPostSaveLayout(ctx, route.Args[0])
	case "daemon/ensure":
		return ensureRestoredAndCapturedOnStartup(ctx)
	case "daemon/bootstrap":
		return bootstrapSidebarDaemonForEnvironment(ctx, r.runtimeEnvironment(), stderr, r.ipcServer, r.direct())
	case "daemon/serve-ui":
		return serveSidebarUIForEnvironment(ctx, r.runtimeEnvironment(), route.Flags, stdout, r.sidebar, r.ipcClient)
	case "hooks/run":
		return runHooksCommand(ctx, route.Args, route.Flags, stdout, stderr)
	case "runtime/self-update":
		return runSelfUpdate(ctx, stdout, stderr)
	case "daemon/serve":
		return serveSidebarDaemonForEnvironment(ctx, r.runtimeEnvironment(), r.ipcServer, r.direct())
	default:
		_, _ = fmt.Fprintf(stderr, "%s not implemented yet\n", route.Path)
		return fmt.Errorf("unimplemented route: %s", route.Path)
	}
}

func errUnhandledRoute(path string) error {
	return fmt.Errorf("unhandled route: %s", path)
}
