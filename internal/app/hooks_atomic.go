package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func writeHookFileAtomic(path string, data []byte, defaultPerm fs.FileMode) error {
	if defaultPerm == 0 {
		defaultPerm = 0o644
	}
	targetPath := path
	perm := defaultPerm.Perm()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				linkTarget, targetErr := symlinkTargetPath(path)
				if targetErr == nil {
					if _, statErr := os.Stat(linkTarget); statErr != nil {
						return fmt.Errorf("stat symlink hook config target %s: %w", linkTarget, statErr)
					}
				}
				return fmt.Errorf("resolve symlink hook config %s: %w", path, err)
			}
			targetPath = resolved
			if targetInfo, err := os.Stat(targetPath); err == nil {
				perm = targetInfo.Mode().Perm()
			} else {
				return fmt.Errorf("stat symlink hook config target %s: %w", targetPath, err)
			}
		} else {
			perm = info.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	needsCleanup := true
	defer func() {
		if needsCleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp hook config %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp hook config %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp hook config %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp hook config %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("replace hook config %s: %w", targetPath, err)
	}
	needsCleanup = false
	syncDirBestEffort(dir)
	return nil
}

func symlinkTargetPath(path string) (string, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(path), target)), nil
}

func syncDirBestEffort(dir string) {
	dirFile, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() { _ = dirFile.Close() }()
	_ = dirFile.Sync()
}
