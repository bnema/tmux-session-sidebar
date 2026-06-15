package runtimefs

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// EnsureRuntimeDirPrivate creates dir with 0o700 if it does not exist.
// Existing paths must already be private enough to trust: the helper returns
// an error for symlinks, non-directories, Unix paths not owned by the current
// user, or directories with unsafe group/world permissions instead of trying
// to repair them in place.
func EnsureRuntimeDirPrivate(dir string) error {
	info, _, err := ensureRuntimeDirInfo(dir)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("runtime path %q is a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("runtime path %q is not a directory", dir)
	}

	if runtime.GOOS != "windows" {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			if int(stat.Uid) != syscall.Geteuid() {
				return fmt.Errorf("runtime path %q is owned by uid %d, not current user %d",
					dir, stat.Uid, syscall.Geteuid())
			}
		}
		// Some Unix variants may not expose syscall.Stat_t through FileInfo.Sys.
		// When that happens, ownership validation is skipped and the remaining
		// directory safety checks still apply.
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("runtime path %q has unsafe permissions %o", dir, info.Mode().Perm())
	}

	return nil
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
