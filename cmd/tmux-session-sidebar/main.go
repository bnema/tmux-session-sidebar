package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/adapters/ipcunix"
	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/app"
)

var (
	signalContext = signal.NotifyContext
	runApp        = app.Run
	newRouter     = func() app.Router {
		tmux := tmuxcli.Client{Process: process.Runner{}}
		socketPath := filepath.Join(app.StateDir(), "sidebar.sock")
		return app.NewRuntimeRouter(tmux, ipcunix.NewClient(socketPath), ipcunix.NewServer(socketPath))
	}
)

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	ctx, stop := signalContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runApp(ctx, args, stdout, stderr, newRouter())
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
