package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/bnema/tmux-session-sidebar/internal/adapters/daemonctl"
	watchfsnotify "github.com/bnema/tmux-session-sidebar/internal/adapters/fsnotify"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/gitcli"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/githubrelease"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/ipcunix"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/locker"
	adapterlogger "github.com/bnema/tmux-session-sidebar/internal/adapters/logger"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/portalsettings"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/process"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/internal/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/app"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/runtimelog"
)

var (
	signalContext = signal.NotifyContext
	runApp        = app.Run
	newRouter     = func(ctx context.Context) app.Router {
		router, _ := buildRuntimeRouter(ctx, process.Runner{})
		return router
	}
)

func buildRuntimeRouter(ctx context.Context, runner ports.ProcessPort) (app.Router, app.RuntimeScope) {
	tmuxClient := tmuxcli.Client{Process: runner}
	git := gitcli.Git{Process: runner}
	app.SetRuntimeDependencies(app.RuntimeDependencies{
		Multiplexer:    tmuxClient,
		Git:            git,
		ReleaseChecker: githubrelease.Client{},
		WatcherFactory: func() ports.FileWatcherPort { return watchfsnotify.Watcher{} },
		StateStoreFactory: func(scope app.RuntimeScope) ports.StateStorePort {
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
		SystemColorSchemePort: portalsettings.ColorSchemeSource{},
		SidebarUI:             uiRunner{},
	})
	scope := app.RuntimeScopeForProcess(ctx, runner)
	app.SetRuntimeScope(scope)
	daemonLauncher := daemonctl.Launcher{Process: runner, StateDir: scope.Dir}
	return app.NewRuntimeRouterWithDaemon(tmuxClient, ipcunix.NewClient(scope.IPCSocketPath), ipcunix.NewServer(scope.IPCSocketPath), daemonLauncher), scope
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	ctx, stop := signalContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runApp(ctx, args, stdout, stderr, newRouter(ctx))
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
