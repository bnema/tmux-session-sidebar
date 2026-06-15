package app

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/bnema/tmux-session-sidebar/ports"
)

// RuntimeDependencies holds the runtime-injected ports and factories used by
// the app layer.
type RuntimeDependencies struct {
	Tmux                  ports.TmuxRuntimePort
	Git                   ports.GitPort
	ReleaseChecker        ports.ReleaseCheckerPort
	WatcherFactory        func() ports.FileWatcherPort
	StateStoreFactory     func(scope RuntimeScope) ports.StateStorePort
	LockerFactory         func(dir string) ports.LockerPort
	ActivityLoggerFactory func(path string, maxBytes int64) (ports.LoggerPort, io.Closer, error)
	LogWriterFactory      func(path string, maxBytes int64) (ports.SyncWriteCloser, error)
	SidebarUI             SidebarUIRunner
}

var runtimeDependenciesState struct {
	mu   sync.RWMutex
	deps RuntimeDependencies
}

// SetRuntimeDependencies installs the runtime dependencies used by the app
// layer.
func SetRuntimeDependencies(deps RuntimeDependencies) {
	runtimeDependenciesState.mu.Lock()
	defer runtimeDependenciesState.mu.Unlock()
	runtimeDependenciesState.deps = deps
}

// ResetRuntimeDependenciesForTest clears the installed runtime dependencies for
// test isolation.
func ResetRuntimeDependenciesForTest() {
	SetRuntimeDependencies(RuntimeDependencies{})
}

func runtimeDependencies() RuntimeDependencies {
	runtimeDependenciesState.mu.RLock()
	defer runtimeDependenciesState.mu.RUnlock()
	return runtimeDependenciesState.deps
}

func runtimeTmux() ports.TmuxRuntimePort {
	deps := runtimeDependencies()
	if deps.Tmux == nil {
		return missingTmuxRuntime{}
	}
	return deps.Tmux
}

func runtimeGit() ports.GitPort {
	deps := runtimeDependencies()
	if deps.Git == nil {
		return missingGit{}
	}
	return deps.Git
}

func runtimeReleaseChecker() ports.ReleaseCheckerPort {
	deps := runtimeDependencies()
	if deps.ReleaseChecker == nil {
		return missingReleaseChecker{}
	}
	return deps.ReleaseChecker
}

func runtimeWatcher() ports.FileWatcherPort {
	deps := runtimeDependencies()
	if deps.WatcherFactory == nil {
		return missingWatcher{}
	}
	return deps.WatcherFactory()
}

func runtimeStateStore(scope RuntimeScope) ports.StateStorePort {
	deps := runtimeDependencies()
	if deps.StateStoreFactory == nil {
		return missingStateStore{}
	}
	return deps.StateStoreFactory(scope)
}

func runtimeLocker(dir string) ports.LockerPort {
	deps := runtimeDependencies()
	if deps.LockerFactory == nil {
		return missingLocker{}
	}
	return deps.LockerFactory(dir)
}

func runtimeActivityLogger(path string, maxBytes int64) (ports.LoggerPort, io.Closer, error) {
	deps := runtimeDependencies()
	if deps.ActivityLoggerFactory == nil {
		return nil, nil, missingDependencyError("activity logger factory")
	}
	return deps.ActivityLoggerFactory(path, maxBytes)
}

func runtimeLogWriter(path string, maxBytes int64) (ports.SyncWriteCloser, error) {
	deps := runtimeDependencies()
	if deps.LogWriterFactory == nil {
		return nil, missingDependencyError("log writer factory")
	}
	return deps.LogWriterFactory(path, maxBytes)
}

func runtimeSidebarUI() SidebarUIRunner {
	deps := runtimeDependencies()
	if deps.SidebarUI == nil {
		return missingSidebarUI{}
	}
	return deps.SidebarUI
}

func missingDependencyError(name string) error {
	return fmt.Errorf("app runtime dependencies missing %s", name)
}

type missingReleaseChecker struct{}

func (missingReleaseChecker) LatestReleaseTag(context.Context) (string, error) {
	return "", missingDependencyError("release checker")
}

type missingWatcher struct{}

func (missingWatcher) Watch(context.Context, []string) (<-chan ports.FileWatchEvent, <-chan error, error) {
	return nil, nil, missingDependencyError("watcher factory")
}

type missingGit struct{}

func (missingGit) RepoRoot(context.Context, string) (string, error) {
	return "", missingDependencyError("git port")
}

func (missingGit) Status(context.Context, string) (ports.GitStatus, error) {
	return ports.GitStatus{}, missingDependencyError("git port")
}

func (missingGit) RepoInfo(context.Context, string) (ports.GitRepoInfo, error) {
	return ports.GitRepoInfo{}, missingDependencyError("git port")
}

func (missingGit) WatchTargets(context.Context, string) (ports.GitWatchTargets, error) {
	return ports.GitWatchTargets{}, missingDependencyError("git port")
}

type missingStateStore struct{}

func (missingStateStore) Load(context.Context, string) (ports.PersistedState, error) {
	return ports.PersistedState{}, missingDependencyError("state store factory")
}

func (missingStateStore) Save(context.Context, string, ports.PersistedState) error {
	return missingDependencyError("state store factory")
}

type missingLocker struct{}

func (missingLocker) Acquire(context.Context, string) (ports.LockHandle, error) {
	return nil, missingDependencyError("locker factory")
}

type missingTmuxRuntime struct{}

func (missingTmuxRuntime) LoadConfig(context.Context) (ports.ConfigSnapshot, error) {
	return ports.ConfigSnapshot{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) ServerID(context.Context) (string, error) {
	return "", missingDependencyError("tmux port")
}

func (missingTmuxRuntime) ListSessions(context.Context) ([]ports.TmuxSessionSnapshot, error) {
	return nil, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) ListClients(context.Context) ([]ports.TmuxClientSnapshot, error) {
	return nil, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) CurrentPanePath(context.Context, string) (string, error) {
	return "", missingDependencyError("tmux port")
}

func (missingTmuxRuntime) SessionPath(context.Context, string) (string, error) {
	return "", missingDependencyError("tmux port")
}

func (missingTmuxRuntime) PaneSize(context.Context, string) (ports.PaneSize, error) {
	return ports.PaneSize{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) SwitchClientSession(context.Context, string, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) DisplayMessage(context.Context, string, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) CreateSession(context.Context, string, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) RenameSession(context.Context, string, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) KillSession(context.Context, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) CloseAfterSwitch(context.Context) (bool, error) {
	return false, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) FindSidebarPane(context.Context, string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) FindSingletonSidebar(context.Context) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) EnsureSingletonSidebar(context.Context, []string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) AttachSingletonSidebar(context.Context, string, string, string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) ParkSingletonSidebar(context.Context, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) RefreshSidebar(context.Context, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) ScheduleSidebarRestoreOnExit(context.Context, string, string) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) LoadSessionMetadata(context.Context, string) (ports.SessionMetadata, error) {
	return ports.SessionMetadata{}, missingDependencyError("tmux port")
}

func (missingTmuxRuntime) SaveSessionMetadata(context.Context, string, ports.SessionMetadata) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) RefreshAllSidebars(context.Context) error {
	return missingDependencyError("tmux port")
}

func (missingTmuxRuntime) Run(context.Context, []string) (ports.Result, error) {
	return ports.Result{}, missingDependencyError("tmux port")
}
