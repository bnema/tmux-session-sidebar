package app

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// EnsureRuntimeDirPrivate creates dir with 0o700 if it does not exist, or
// tightens an existing directory's permissions to 0o700. It returns an error
// if the path exists and is not a directory, if it is a symlink, if the
// directory is not owned by the current user (on Unix), or if permissions
// cannot be tightened to the required private state.
func EnsureRuntimeDirPrivate(dir string) error {
	info, created, err := ensureRuntimeDirInfo(dir)
	if err != nil {
		return err
	}

	// Reject symlinks — following them into an unexpected location and then
	// tightening permissions would be unsafe.
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("runtime path %q is a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("runtime path %q is not a directory", dir)
	}

	// Validate current-user ownership on Unix before chmod, so we fail
	// closed even when running as root.
	if runtime.GOOS != "windows" {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			if int(stat.Uid) != syscall.Geteuid() {
				return fmt.Errorf("runtime path %q is owned by uid %d, not current user %d",
					dir, stat.Uid, syscall.Geteuid())
			}
		}
	}

	if !created && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("runtime path %q has unsafe permissions %o", dir, info.Mode().Perm())
	}

	return os.Chmod(dir, 0o700)
}

func ensureRuntimeDirInfo(dir string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(dir)
	if err == nil {
		return info, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return nil, true, err
	}
	return info, true, nil
}
