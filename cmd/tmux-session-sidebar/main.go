package main

import (
	"context"
	"os"

	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/internal/app"
)

func main() {
	tmux := tmuxcli.Client{Process: process.Runner{}}
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, app.NewRouter(tmux)))
}
