package app

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

// RuntimeDependencies holds the runtime-injected ports and factories used by
// the app layer.
type RuntimeDependencies struct {
	Multiplexer           ports.RuntimePort
	Git                   ports.GitPort
	ReleaseChecker        ports.ReleaseCheckerPort
	WatcherFactory        func() ports.FileWatcherPort
	StateStoreFactory     func(scope RuntimeScope) ports.StateStorePort
	LockerFactory         func(dir string) ports.LockerPort
	ActivityLoggerFactory func(path string, maxBytes int64) (ports.LoggerPort, io.Closer, error)
	LogWriterFactory      func(path string, maxBytes int64) (ports.SyncWriteCloser, error)
	SystemColorSchemePort ports.SystemColorSchemePort
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

func runtimeMultiplexer() ports.RuntimePort {
	deps := runtimeDependencies()
	if deps.Multiplexer == nil {
		return missingRuntime{}
	}
	return deps.Multiplexer
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

func runtimeSystemColorScheme() ports.SystemColorSchemePort {
	deps := runtimeDependencies()
	if deps.SystemColorSchemePort == nil {
		return missingSystemColorScheme{}
	}
	return deps.SystemColorSchemePort
}

func missingDependencyError(name string) error {
	return fmt.Errorf("app runtime dependencies missing %s", name)
}

type missingReleaseChecker struct{}

type missingSystemColorScheme struct{}

func (missingSystemColorScheme) CurrentPreference(context.Context) (config.SystemColorSchemePreference, error) {
	return config.SystemColorSchemeNoPreference, missingDependencyError("system color scheme port")
}

func (missingSystemColorScheme) Watch(context.Context) (<-chan config.SystemColorSchemePreference, <-chan error, error) {
	return nil, nil, missingDependencyError("system color scheme port")
}

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

type missingRuntime struct{}

func (missingRuntime) LoadConfig(context.Context) (ports.ConfigSnapshot, error) {
	return ports.ConfigSnapshot{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) ServerID(context.Context) (string, error) {
	return "", missingDependencyError("multiplexer port")
}

func (missingRuntime) ListSessions(context.Context) ([]ports.SessionSnapshot, error) {
	return nil, missingDependencyError("multiplexer port")
}

func (missingRuntime) ListClients(context.Context) ([]ports.ClientSnapshot, error) {
	return nil, missingDependencyError("multiplexer port")
}

func (missingRuntime) CurrentPanePath(context.Context, string) (string, error) {
	return "", missingDependencyError("multiplexer port")
}

func (missingRuntime) SessionPath(context.Context, string) (string, error) {
	return "", missingDependencyError("multiplexer port")
}

func (missingRuntime) SessionPaths(context.Context, []string) (map[string]string, error) {
	return nil, missingDependencyError("multiplexer port")
}

func (missingRuntime) PaneSize(context.Context, string) (ports.PaneSize, error) {
	return ports.PaneSize{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) SwitchClientSession(context.Context, string, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) DisplayMessage(context.Context, string, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) CreateSession(context.Context, string, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) RenameSession(context.Context, string, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) KillSession(context.Context, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) CloseAfterSwitch(context.Context) (bool, error) {
	return false, missingDependencyError("multiplexer port")
}

func (missingRuntime) FindSidebarPane(context.Context, string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) FindSingletonSidebar(context.Context) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) EnsureSingletonSidebar(context.Context, []string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) AttachSingletonSidebar(context.Context, string, string, string) (ports.PaneRef, error) {
	return ports.PaneRef{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) ParkSingletonSidebar(context.Context, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) RefreshSidebar(context.Context, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) ScheduleSidebarRestoreOnExit(context.Context, string, string) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) LoadSessionMetadata(context.Context, string) (ports.SessionMetadata, error) {
	return ports.SessionMetadata{}, missingDependencyError("multiplexer port")
}

func (missingRuntime) SaveSessionMetadata(context.Context, string, ports.SessionMetadata) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) RefreshAllSidebars(context.Context) error {
	return missingDependencyError("multiplexer port")
}

func (missingRuntime) Run(context.Context, []string) (ports.Result, error) {
	return ports.Result{}, missingDependencyError("multiplexer port")
}
