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
	perm := defaultPerm.Perm()
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
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
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace hook config %s: %w", path, err)
	}
	needsCleanup = false
	syncDirBestEffort(dir)
	return nil
}

func syncDirBestEffort(dir string) {
	dirFile, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() { _ = dirFile.Close() }()
	_ = dirFile.Sync()
}
