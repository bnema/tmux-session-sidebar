package app

import (
	"os"
	"slices"
)

func init() {
	_ = os.Unsetenv("TMUX")
}

func matchesDaemonServeUICommand() func([]string) bool {
	return func(command []string) bool {
		return slices.Contains(command, "daemon") && slices.Contains(command, "serve-ui")
	}
}
