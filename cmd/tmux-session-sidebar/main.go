package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/app"
)

var (
	signalContext = signal.NotifyContext
	runApp        = app.Run
	newRouter     = func() app.Router {
		tmux := tmuxcli.Client{Process: process.Runner{}}
		return app.NewRouter(tmux)
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
