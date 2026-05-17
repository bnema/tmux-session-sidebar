package filesystem

import (
	"os"
	"path/filepath"
)

type FS struct{}

func (FS) ResolvePath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (FS) ListImmediateDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	dirCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			dirCount++
		}
	}
	dirs := make([]string, 0, dirCount)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	return dirs, nil
}
