package app

import (
	"context"
	"io"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

// RuntimeEnvironment carries the runtime dependencies and filesystem scope for
// one composed sidebar runtime. New production composition should pass this
// value explicitly instead of mutating the package-level compatibility shims.
type RuntimeEnvironment struct {
	Dependencies RuntimeDependencies
	Scope        RuntimeScope
}

func NewRuntimeEnvironment(deps RuntimeDependencies, scope RuntimeScope) RuntimeEnvironment {
	return RuntimeEnvironment{Dependencies: deps, Scope: scope}
}

func currentRuntimeEnvironment() RuntimeEnvironment {
	return RuntimeEnvironment{Dependencies: runtimeDependencies(), Scope: CurrentRuntimeScope()}
}

func (env RuntimeEnvironment) runtimeMultiplexer() ports.RuntimePort {
	if env.Dependencies.Multiplexer == nil {
		return missingRuntime{}
	}
	return env.Dependencies.Multiplexer
}

func (env RuntimeEnvironment) runtimeGit() ports.GitPort {
	if env.Dependencies.Git == nil {
		return missingGit{}
	}
	return env.Dependencies.Git
}

func (env RuntimeEnvironment) runtimeFilesystem() ports.FilesystemPort {
	if env.Dependencies.Filesystem == nil {
		return missingFilesystem{}
	}
	return env.Dependencies.Filesystem
}

func (env RuntimeEnvironment) runtimeReleaseChecker() ports.ReleaseCheckerPort {
	if env.Dependencies.ReleaseChecker == nil {
		return missingReleaseChecker{}
	}
	return env.Dependencies.ReleaseChecker
}

func (env RuntimeEnvironment) runtimeWatcher() ports.FileWatcherPort {
	if env.Dependencies.WatcherFactory == nil {
		return missingWatcher{}
	}
	return env.Dependencies.WatcherFactory()
}

func (env RuntimeEnvironment) runtimeStateStore(scope RuntimeScope) ports.StateStorePort {
	if env.Dependencies.StateStoreFactory == nil {
		return missingStateStore{}
	}
	return env.Dependencies.StateStoreFactory(scope)
}

func (env RuntimeEnvironment) runtimeLocker(dir string) ports.LockerPort {
	if env.Dependencies.LockerFactory == nil {
		return missingLocker{}
	}
	return env.Dependencies.LockerFactory(dir)
}

func (env RuntimeEnvironment) runtimeActivityLogger(path string, maxBytes int64) (ports.LoggerPort, io.Closer, error) {
	if env.Dependencies.ActivityLoggerFactory == nil {
		return nil, nil, missingDependencyError("activity logger factory")
	}
	return env.Dependencies.ActivityLoggerFactory(path, maxBytes)
}

func (env RuntimeEnvironment) runtimeLogWriter(path string, maxBytes int64) (ports.SyncWriteCloser, error) {
	if env.Dependencies.LogWriterFactory == nil {
		return nil, missingDependencyError("log writer factory")
	}
	return env.Dependencies.LogWriterFactory(path, maxBytes)
}

func (env RuntimeEnvironment) runtimeSidebarUI() SidebarUIRunner {
	if env.Dependencies.SidebarUI == nil {
		return missingSidebarUI{}
	}
	return env.Dependencies.SidebarUI
}

func (env RuntimeEnvironment) runtimeSystemColorScheme() ports.SystemColorSchemePort {
	if env.Dependencies.SystemColorSchemePort == nil {
		return missingSystemColorScheme{}
	}
	return env.Dependencies.SystemColorSchemePort
}

func (env RuntimeEnvironment) currentRuntimeScope() RuntimeScope {
	if env.Scope == (RuntimeScope{}) {
		return RuntimeScopeForProcess(context.Background(), nil)
	}
	return env.Scope
}

func (env RuntimeEnvironment) isZero() bool {
	return env.Scope == (RuntimeScope{}) &&
		env.Dependencies.Multiplexer == nil &&
		env.Dependencies.Git == nil &&
		env.Dependencies.Filesystem == nil &&
		env.Dependencies.ReleaseChecker == nil &&
		env.Dependencies.WatcherFactory == nil &&
		env.Dependencies.StateStoreFactory == nil &&
		env.Dependencies.LockerFactory == nil &&
		env.Dependencies.ActivityLoggerFactory == nil &&
		env.Dependencies.LogWriterFactory == nil &&
		env.Dependencies.SystemColorSchemePort == nil &&
		env.Dependencies.SidebarUI == nil
}
