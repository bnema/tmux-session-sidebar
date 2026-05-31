package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	watchfsnotify "github.com/bnema/tmux-session-sidebar/adapters/fsnotify"
	"github.com/bnema/tmux-session-sidebar/adapters/gitcli"
	"github.com/bnema/tmux-session-sidebar/adapters/gitgo"
	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/ports"
)

const defaultMetadataGitStatusConcurrency = 4

type MetadataService struct {
	Store                ports.StateStorePort
	Tmux                 ports.TmuxQueryPort
	Git                  ports.GitPort
	Watcher              ports.FileWatcherPort
	Refresher            ports.SidebarRefresherPort
	ReconcileRequests    <-chan struct{}
	LockStore            func(context.Context, func(ports.StateStorePort) error) error
	Debounce             time.Duration
	ReconcileInterval    time.Duration
	GitStatusTimeout     time.Duration
	GitStatusConcurrency int
}

type MetadataRepoSubscription struct {
	RepoRoot     string
	WorktreeRoot string
	SessionNames []string
	WatchFiles   []string
	WatchDirs    []string
}

func NewMetadataService() *MetadataService {
	tmux := newTmuxClient()
	return &MetadataService{
		Store:                sessionOrderStore(),
		Tmux:                 tmux,
		Git:                  gitgo.Git{Fallback: gitcli.Git{Process: process.Runner{}}},
		Watcher:              watchfsnotify.Watcher{},
		Refresher:            tmux,
		LockStore:            defaultMetadataLockStore,
		Debounce:             250 * time.Millisecond,
		ReconcileInterval:    time.Minute,
		GitStatusTimeout:     metadataGitStatusTimeout,
		GitStatusConcurrency: defaultMetadataGitStatusConcurrency,
	}
}

func defaultMetadataLockStore(ctx context.Context, fn func(ports.StateStorePort) error) error {
	return withLockedSidebarStore(ctx, func(store storefs.Store) error {
		return fn(store)
	})
}

func (s *MetadataService) CaptureAndRefresh(ctx context.Context, cfg ports.ConfigSnapshot) error {
	changed, err := s.Capture(ctx, cfg)
	if err != nil {
		return err
	}
	if changed && s.Refresher != nil {
		return s.Refresher.RefreshAllSidebars(ctx)
	}
	return nil
}

func (s *MetadataService) Capture(ctx context.Context, cfg ports.ConfigSnapshot) (bool, error) {
	if !cfg.MetadataSublineEnabled {
		return false, nil
	}
	if s.Store == nil || s.Tmux == nil || s.Git == nil {
		return false, errors.New("metadata service missing dependency")
	}
	lockStore := s.LockStore
	if lockStore == nil {
		lockStore = func(ctx context.Context, fn func(ports.StateStorePort) error) error { return fn(s.Store) }
	}
	gitStatusTimeout := s.GitStatusTimeout
	if gitStatusTimeout <= 0 {
		gitStatusTimeout = metadataGitStatusTimeout
	}
	concurrency := s.GitStatusConcurrency
	if concurrency <= 0 {
		concurrency = defaultMetadataGitStatusConcurrency
	}

	state, err := s.Store.Load(ctx, "tmux")
	if err != nil {
		return false, err
	}
	live, err := s.Tmux.ListSessions(ctx)
	if err != nil {
		return false, err
	}

	livePaths := make(map[string]string, len(live))
	for _, session := range live {
		if path, err := s.Tmux.SessionPath(ctx, session.Name); err == nil {
			livePaths[session.Name] = path
		}
	}
	liveNames := make(map[string]struct{}, len(live))
	sessionPaths := make(map[string]string, len(live))
	terminalDeletes := make(map[string]struct{})
	for _, session := range live {
		liveNames[session.Name] = struct{}{}
		path, ok := sessionMetadataCapturePath(session.Name, state.Sessions[session.Name], livePaths)
		if !ok {
			terminalDeletes[session.Name] = struct{}{}
			continue
		}
		sessionPaths[session.Name] = path
	}

	results := make(map[string]ports.GitStatus, len(sessionPaths))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for sessionName, path := range sessionPaths {
		wg.Add(1)
		go func(sessionName string, path string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			statusCtx, cancel := context.WithTimeout(ctx, gitStatusTimeout)
			defer cancel()
			status, err := s.Git.Status(statusCtx, path)
			if err != nil {
				if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
					mu.Lock()
					terminalDeletes[sessionName] = struct{}{}
					mu.Unlock()
					return
				}
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: git status failed for session %q path %q: %v\n", sessionName, path, err)
				return
			}
			mu.Lock()
			results[sessionName] = status
			mu.Unlock()
		}(sessionName, path)
	}
	wg.Wait()

	changed := false
	err = lockStore(ctx, func(store ports.StateStorePort) error {
		latest, err := store.Load(ctx, "tmux")
		if err != nil {
			return err
		}
		next := make(map[string]ports.GitStatus, len(liveNames))
		for name := range liveNames {
			if _, ok := terminalDeletes[name]; ok {
				continue
			}
			if status, ok := results[name]; ok {
				next[name] = status
				continue
			}
			if status, ok := latest.Metadata[name]; ok {
				next[name] = status
			}
		}
		if gitMetadataEqual(latest.Metadata, next) {
			return nil
		}
		latest.Metadata = next
		changed = true
		return store.Save(ctx, "tmux", latest)
	})
	return changed, err
}

var errMetadataReconcile = errors.New("metadata reconcile requested")
var errMetadataFallback = errors.New("metadata watcher fallback")

func (s *MetadataService) Run(ctx context.Context, cfg ports.ConfigSnapshot) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.runWatchOnce(ctx, cfg)
		switch {
		case errors.Is(err, errMetadataReconcile):
			continue
		case errors.Is(err, errMetadataFallback):
			if err := s.runFallbackPoll(ctx, cfg); errors.Is(err, errMetadataReconcile) {
				continue
			} else {
				return err
			}
		default:
			return err
		}
	}
}

func (s *MetadataService) runWatchOnce(ctx context.Context, cfg ports.ConfigSnapshot) error {
	if !cfg.MetadataSublineEnabled {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := s.CaptureAndRefresh(ctx, cfg); err != nil {
		return err
	}
	subs, err := s.Reconcile(ctx, cfg)
	if err != nil {
		return err
	}
	if len(subs) == 0 || s.Watcher == nil {
		return errMetadataFallback
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	paths := watchPaths(subs)
	events, errs, err := s.Watcher.Watch(watchCtx, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watcher setup failed: %v\n", err)
		return errMetadataFallback
	}
	debounce := s.Debounce
	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	reconcileInterval := s.ReconcileInterval
	if reconcileInterval <= 0 {
		reconcileInterval = time.Minute
	}
	reconcileTimer := time.NewTimer(reconcileInterval)
	defer reconcileTimer.Stop()
	pending := map[string]MetadataRepoSubscription{}
	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer = nil
		timerC = nil
	}
	defer stopTimer()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reconcileTimer.C:
			return errMetadataReconcile
		case <-s.ReconcileRequests:
			return errMetadataReconcile
		case event, ok := <-events:
			if !ok {
				return errMetadataFallback
			}
			matched := false
			for repo, sub := range subs {
				if metadataEventMatchesSubscription(event.Path, sub) {
					pending[repo] = sub
					matched = true
				}
			}
			if matched {
				stopTimer()
				timer = time.NewTimer(debounce)
				timerC = timer.C
			}
		case err, ok := <-errs:
			if ok && err != nil {
				fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watcher failed: %v\n", err)
				return errMetadataFallback
			}
		case <-timerC:
			for repo, sub := range pending {
				if _, err := s.CaptureRepo(ctx, sub); err != nil && !errors.Is(err, context.Canceled) {
					fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata repo capture failed for %q: %v\n", repo, err)
				}
			}
			pending = map[string]MetadataRepoSubscription{}
			timer = nil
			timerC = nil
		}
	}
}

func (s *MetadataService) runFallbackPoll(ctx context.Context, cfg ports.ConfigSnapshot) error {
	interval := s.ReconcileInterval
	if interval <= 0 {
		interval = time.Minute
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ReconcileRequests:
		return errMetadataReconcile
	case <-timer.C:
		if err := s.CaptureAndRefresh(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata fallback capture failed: %v\n", err)
		}
		return errMetadataReconcile
	}
}

func (s *MetadataService) Reconcile(ctx context.Context, cfg ports.ConfigSnapshot) (map[string]MetadataRepoSubscription, error) {
	if !cfg.MetadataSublineEnabled {
		return nil, nil
	}
	if s.Store == nil || s.Tmux == nil || s.Git == nil {
		return nil, errors.New("metadata service missing dependency")
	}
	state, err := s.Store.Load(ctx, "tmux")
	if err != nil {
		return nil, err
	}
	live, err := s.Tmux.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	subs := map[string]MetadataRepoSubscription{}
	for _, session := range live {
		path, pathErr := s.Tmux.SessionPath(ctx, session.Name)
		if pathErr != nil || path == "" {
			var ok bool
			path, ok = sessionMetadataPath(state.Sessions[session.Name])
			if !ok {
				continue
			}
		}
		info, err := s.Git.RepoInfo(ctx, path)
		if err != nil {
			if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
				continue
			}
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata repo info failed for session %q path %q: %v\n", session.Name, path, err)
			continue
		}
		targets, err := s.Git.WatchTargets(ctx, path)
		if err != nil {
			if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
				continue
			}
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watch targets failed for session %q path %q: %v\n", session.Name, path, err)
			continue
		}
		sub := subs[info.RepoRoot]
		if sub.RepoRoot == "" {
			sub.RepoRoot = info.RepoRoot
			sub.WorktreeRoot = info.WorktreeRoot
		}
		sub.SessionNames = append(sub.SessionNames, session.Name)
		sub.WatchFiles = appendUniqueStrings(sub.WatchFiles, targets.Files...)
		sub.WatchDirs = appendUniqueStrings(sub.WatchDirs, targets.Dirs...)
		subs[info.RepoRoot] = sub
	}
	return subs, nil
}

func (s *MetadataService) gitStatusTimeout() time.Duration {
	if s.GitStatusTimeout > 0 {
		return s.GitStatusTimeout
	}
	return metadataGitStatusTimeout
}

func (s *MetadataService) CaptureRepo(ctx context.Context, sub MetadataRepoSubscription) (bool, error) {
	if s.Store == nil || s.Git == nil {
		return false, errors.New("metadata service missing dependency")
	}
	statusCtx, cancel := context.WithTimeout(ctx, s.gitStatusTimeout())
	defer cancel()
	status, err := s.Git.Status(statusCtx, sub.WorktreeRoot)
	terminalDelete := false
	if err != nil {
		if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
			terminalDelete = true
		} else {
			return false, err
		}
	}
	lockStore := s.LockStore
	if lockStore == nil {
		lockStore = func(ctx context.Context, fn func(ports.StateStorePort) error) error { return fn(s.Store) }
	}
	changed := false
	err = lockStore(ctx, func(store ports.StateStorePort) error {
		latest, err := store.Load(ctx, "tmux")
		if err != nil {
			return err
		}
		next := cloneGitMetadata(latest.Metadata)
		for _, name := range sub.SessionNames {
			if terminalDelete {
				delete(next, name)
				continue
			}
			next[name] = status
		}
		if gitMetadataEqual(latest.Metadata, next) {
			return nil
		}
		latest.Metadata = next
		changed = true
		return store.Save(ctx, "tmux", latest)
	})
	if err != nil {
		return false, err
	}
	if changed && s.Refresher != nil {
		return true, s.Refresher.RefreshAllSidebars(ctx)
	}
	return changed, nil
}

func watchPaths(subs map[string]MetadataRepoSubscription) []string {
	paths := []string{}
	for _, sub := range subs {
		paths = appendUniqueStrings(paths, sub.WatchFiles...)
		paths = appendUniqueStrings(paths, sub.WatchDirs...)
	}
	return paths
}

func metadataEventMatchesSubscription(path string, sub MetadataRepoSubscription) bool {
	path = filepath.Clean(path)
	for _, file := range sub.WatchFiles {
		if path == filepath.Clean(file) {
			return true
		}
	}
	for _, dir := range sub.WatchDirs {
		dir = filepath.Clean(dir)
		if path == dir {
			return true
		}
		if metadataDirWatchIsRecursive(dir, sub) {
			if strings.HasPrefix(path, dir+string(os.PathSeparator)) {
				return true
			}
			continue
		}
		if filepath.Dir(path) == dir {
			return true
		}
	}
	return false
}

func metadataDirWatchIsRecursive(dir string, sub MetadataRepoSubscription) bool {
	return dir == filepath.Clean(sub.WorktreeRoot) || filepath.Base(dir) == "refs"
}

func appendUniqueStrings(values []string, next ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(next))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range next {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func cloneGitMetadata(metadata map[string]ports.GitStatus) map[string]ports.GitStatus {
	if metadata == nil {
		return map[string]ports.GitStatus{}
	}
	cloned := make(map[string]ports.GitStatus, len(metadata))
	maps.Copy(cloned, metadata)
	return cloned
}
