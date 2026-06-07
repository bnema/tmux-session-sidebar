package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/bnema/tmux-session-sidebar/adapters/daemonctl"
	"github.com/bnema/tmux-session-sidebar/adapters/ipcunix"
	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/app"
	"github.com/bnema/tmux-session-sidebar/ports"
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
	tmux := tmuxcli.Client{Process: runner}
	scope := app.RuntimeScopeForProcess(ctx, runner)
	app.SetRuntimeScope(scope)
	daemonLauncher := daemonctl.Launcher{Process: runner, StateDir: scope.Dir}
	return app.NewRuntimeRouterWithDaemon(tmux, ipcunix.NewClient(scope.IPCSocketPath), ipcunix.NewServer(scope.IPCSocketPath), daemonLauncher), scope
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	ctx, stop := signalContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runApp(ctx, args, stdout, stderr, newRouter(ctx))
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
