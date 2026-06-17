package app

import (
	"context"
	"io"
	"os"
	"testing"

	watchfsnotify "github.com/bnema/tmux-session-sidebar/adapters/fsnotify"
	"github.com/bnema/tmux-session-sidebar/adapters/gitcli"
	"github.com/bnema/tmux-session-sidebar/adapters/githubrelease"
	"github.com/bnema/tmux-session-sidebar/adapters/locker"
	adapterlogger "github.com/bnema/tmux-session-sidebar/adapters/logger"
	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/runtimelog"
	"github.com/bnema/tmux-session-sidebar/internal/viewmodel"
)

type testSidebarUIRunner struct{}

func (testSidebarUIRunner) Run(context.Context, []viewmodel.TreeItem, SidebarUIActions, SidebarUIOptions, io.Writer) error {
	return nil
}

func TestMain(m *testing.M) {
	runner := process.Runner{}
	tmux := tmuxcli.Client{Process: runner}
	git := gitcli.Git{Process: runner}
	// This wiring intentionally stays local to app tests so the test package does
	// not depend on the cmd composition root.
	SetRuntimeDependencies(RuntimeDependencies{
		Multiplexer:    tmux,
		Git:            git,
		ReleaseChecker: githubrelease.Client{},
		WatcherFactory: func() ports.FileWatcherPort { return watchfsnotify.Watcher{} },
		StateStoreFactory: func(scope RuntimeScope) ports.StateStorePort {
			return storefs.New(scope.StateDir)
		},
		LockerFactory: func(dir string) ports.LockerPort {
			return locker.FileLocker{Dir: dir}
		},
		ActivityLoggerFactory: func(path string, maxBytes int64) (ports.LoggerPort, io.Closer, error) {
			writer, err := runtimelog.NewWriter(path, maxBytes)
			if err != nil {
				return nil, nil, err
			}
			return adapterlogger.Logger{Out: writer}, writer, nil
		},
		LogWriterFactory: func(path string, maxBytes int64) (ports.SyncWriteCloser, error) {
			return runtimelog.NewWriter(path, maxBytes)
		},
		SidebarUI: testSidebarUIRunner{},
	})
	code := m.Run()
	ResetRuntimeDependenciesForTest()
	os.Exit(code)
}
