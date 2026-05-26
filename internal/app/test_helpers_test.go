package app

import "slices"

func matchesDaemonServeUICommand() func([]string) bool {
	return func(command []string) bool {
		return slices.Contains(command, "daemon") && slices.Contains(command, "serve-ui")
	}
}
