package app

import "github.com/bnema/tmux-session-sidebar/internal/runtimefs"

func EnsureRuntimeDirPrivate(dir string) error {
	return runtimefs.EnsureRuntimeDirPrivate(dir)
}
