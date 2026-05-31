package fsnotify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gofsnotify "github.com/fsnotify/fsnotify"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Event = ports.FileWatchEvent

type Watcher struct {
	ExcludeDirs []string
}

func (w Watcher) Watch(ctx context.Context, paths []string) (<-chan Event, <-chan error, error) {
	watcher, err := gofsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	events := make(chan Event, 64)
	errs := make(chan error, 8)
	state := watchState{watcher: watcher, excludes: w.excludes()}
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		allowExcluded := state.excluded(cleaned)
		if allowExcluded {
			state.includedExcluded = append(state.includedExcluded, cleaned)
		}
		if err := state.addPath(cleaned, allowExcluded); err != nil {
			_ = watcher.Close()
			return nil, nil, err
		}
	}
	go state.run(ctx, events, errs)
	return events, errs, nil
}

func (w Watcher) excludes() []string {
	if len(w.ExcludeDirs) > 0 {
		return w.ExcludeDirs
	}
	return []string{".git", "node_modules", "target", "dist", "build", ".next", ".cache"}
}

type watchState struct {
	watcher          *gofsnotify.Watcher
	excludes         []string
	includedExcluded []string
}

func (s watchState) run(ctx context.Context, out chan<- Event, errs chan<- error) {
	defer close(out)
	defer close(errs)
	defer func() { _ = s.watcher.Close() }()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			path := filepath.Clean(event.Name)
			if s.excluded(path) && !s.included(path) {
				continue
			}
			if event.Has(gofsnotify.Create) {
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					if err := s.addPath(path, s.included(path)); err != nil {
						sendErr(ctx, errs, err)
					}
				}
			}
			select {
			case out <- Event{Path: path, Op: event.Op.String()}:
			case <-ctx.Done():
				return
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			sendErr(ctx, errs, err)
		}
	}
}

func sendErr(ctx context.Context, errs chan<- error, err error) {
	select {
	case errs <- err:
	case <-ctx.Done():
	}
}

func (s watchState) addPath(path string, allowExcluded bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() || allowExcluded && !isGitRefsPath(path) {
		return s.watcher.Add(path)
	}
	return filepath.WalkDir(path, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if !allowExcluded && s.excluded(current) {
			return filepath.SkipDir
		}
		return s.watcher.Add(current)
	})
}

func isGitRefsPath(path string) bool {
	path = filepath.Clean(path)
	return filepath.Base(path) == "refs" || strings.Contains(path, string(os.PathSeparator)+"refs"+string(os.PathSeparator))
}

func (s watchState) included(path string) bool {
	path = filepath.Clean(path)
	for _, root := range s.includedExcluded {
		root = filepath.Clean(root)
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (s watchState) excluded(path string) bool {
	base := filepath.Base(path)
	if slices.Contains(s.excludes, base) {
		return true
	}
	for part := range strings.SplitSeq(filepath.Clean(path), string(os.PathSeparator)) {
		if slices.Contains(s.excludes, part) {
			return true
		}
	}
	return false
}
