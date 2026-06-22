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

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

const (
	defaultMetadataGitStatusConcurrency   = 4
	defaultMetadataCaptureFailureCooldown = 30 * time.Second
)

type MetadataService struct {
	Store                  ports.StateStorePort
	Query                  ports.QueryPort
	Config                 ports.ConfigPort
	Git                    ports.GitPort
	Watcher                ports.FileWatcherPort
	Refresher              ports.SidebarRefresherPort
	ReconcileRequests      <-chan struct{}
	LockStore              func(context.Context, func(ports.StateStorePort) error) error
	Debounce               time.Duration
	ReconcileInterval      time.Duration
	GitStatusTimeout       time.Duration
	GitStatusConcurrency   int
	CaptureFailureCooldown time.Duration
	Now                    func() time.Time
	captureFailureMu       sync.Mutex
	captureFailureUntil    map[string]time.Time
}

type MetadataRepoSubscription struct {
	RepoRoot     string
	WorktreeRoot string
	SessionNames []string
	WatchFiles   []string
	WatchDirs    []string
}

type metadataSessionPathsQuery interface {
	SessionPaths(ctx context.Context, sessionNames []string) (map[string]string, error)
}

func NewMetadataService() *MetadataService {
	tmux := runtimeMultiplexer()
	return &MetadataService{
		Store:                  sessionOrderStore(),
		Query:                  tmux,
		Config:                 tmux,
		Git:                    runtimeGit(),
		Watcher:                runtimeWatcher(),
		Refresher:              tmux,
		LockStore:              defaultMetadataLockStore,
		Debounce:               250 * time.Millisecond,
		ReconcileInterval:      time.Minute,
		GitStatusTimeout:       metadataGitStatusTimeout,
		GitStatusConcurrency:   defaultMetadataGitStatusConcurrency,
		CaptureFailureCooldown: defaultMetadataCaptureFailureCooldown,
	}
}

func defaultMetadataLockStore(ctx context.Context, fn func(ports.StateStorePort) error) error {
	return withLockedSidebarStore(ctx, func(store scopedStateStore) error {
		return fn(store)
	})
}

func (s *MetadataService) CaptureAndRefresh(ctx context.Context, cfg ports.ConfigSnapshot) error {
	_, err := s.capture(ctx, cfg, func() error {
		if s.Refresher == nil {
			return nil
		}
		return s.Refresher.RefreshAllSidebars(ctx)
	})
	return err
}

func (s *MetadataService) Capture(ctx context.Context, cfg ports.ConfigSnapshot) (bool, error) {
	return s.capture(ctx, cfg, nil)
}

func (s *MetadataService) capture(ctx context.Context, cfg ports.ConfigSnapshot, onChange func() error) (changed bool, err error) {
	if !cfg.MetadataSublineEnabled {
		return false, nil
	}
	if s.Store == nil || s.Query == nil || s.Git == nil {
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
	live, err := s.Query.ListSessions(ctx)
	if err != nil {
		return false, err
	}

	livePaths := metadataLiveSessionPaths(ctx, s.Query, live)
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

	var changedMu sync.Mutex
	markChanged := func() {
		changedMu.Lock()
		changed = true
		changedMu.Unlock()
	}
	notifier := newMetadataChangeNotifier(onChange)
	defer func() {
		if closeErr := notifier.CloseAndWait(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	captureCtx, cancelCapture := context.WithCancel(ctx)
	defer cancelCapture()
	jobs := make(chan metadataCaptureJob)
	results := make(chan metadataCaptureResult)
	var wg sync.WaitGroup
	workerCount := min(max(concurrency, 1), max(len(sessionPaths), 1))
	for range workerCount {
		wg.Go(func() {
			for job := range jobs {
				statusCtx, cancel := context.WithTimeout(captureCtx, gitStatusTimeout)
				status, err := s.Git.Status(statusCtx, job.Path)
				cancel()
				select {
				case results <- metadataCaptureResult{SessionName: job.SessionName, Path: job.Path, Status: status, Err: err}:
				case <-captureCtx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for sessionName, path := range sessionPaths {
			select {
			case jobs <- metadataCaptureJob{SessionName: sessionName, Path: path}:
			case <-captureCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	deletes := make(map[string]struct{}, len(terminalDeletes))
	for name := range terminalDeletes {
		deletes[name] = struct{}{}
	}
	statuses := make(map[string]ports.GitStatus)

	for result := range results {
		if result.Err != nil {
			if errors.Is(result.Err, ports.ErrNotGitRepository) || errors.Is(result.Err, ports.ErrGitPathMissing) {
				deletes[result.SessionName] = struct{}{}
				continue
			}
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: git status failed for session %q path %q: %v\n", result.SessionName, result.Path, result.Err)
			continue
		}
		statuses[result.SessionName] = result.Status
	}
	if err := ctx.Err(); err != nil {
		return changed, err
	}
	if didChange, err := s.batchMetadataSave(ctx, lockStore, liveNames, deletes, statuses); err != nil {
		return changed, err
	} else if didChange {
		markChanged()
		notifier.Signal()
	}
	return changed, nil
}

func newMetadataChangeNotifier(onChange func() error) *metadataChangeNotifier {
	notifier := &metadataChangeNotifier{onChange: onChange}
	if onChange == nil {
		return notifier
	}
	notifier.signals = make(chan struct{}, 1)
	notifier.done = make(chan struct{})
	go notifier.run()
	return notifier
}

type metadataChangeNotifier struct {
	onChange func() error
	signals  chan struct{}
	done     chan struct{}
	errMu    sync.Mutex
	err      error
}

func (n *metadataChangeNotifier) Signal() {
	if n == nil || n.onChange == nil {
		return
	}
	select {
	case n.signals <- struct{}{}:
	default:
	}
}

func (n *metadataChangeNotifier) CloseAndWait() error {
	if n == nil || n.onChange == nil {
		return nil
	}
	close(n.signals)
	<-n.done
	n.errMu.Lock()
	defer n.errMu.Unlock()
	return n.err
}

func (n *metadataChangeNotifier) run() {
	defer close(n.done)
	for range n.signals {
		if err := n.onChange(); err != nil {
			n.errMu.Lock()
			if n.err == nil {
				n.err = err
			}
			n.errMu.Unlock()
		}
	}
}

type metadataCaptureJob struct {
	SessionName string
	Path        string
}

type metadataCaptureResult struct {
	SessionName string
	Path        string
	Status      ports.GitStatus
	Err         error
}

func (s *MetadataService) batchMetadataSave(ctx context.Context, lockStore func(context.Context, func(ports.StateStorePort) error) error, liveNames map[string]struct{}, deletes map[string]struct{}, statuses map[string]ports.GitStatus) (bool, error) {
	changed := false
	err := lockStore(ctx, func(store ports.StateStorePort) error {
		latest, err := store.Load(ctx, "tmux")
		if err != nil {
			return err
		}
		next := liveMetadata(latest.Metadata, liveNames)
		for name := range deletes {
			delete(next, name)
		}
		maps.Copy(next, statuses)
		if gitMetadataEqual(latest.Metadata, next) {
			return nil
		}
		latest.Metadata = next
		changed = true
		return store.Save(ctx, "tmux", latest)
	})
	return changed, err
}

func liveMetadata(current map[string]ports.GitStatus, liveNames map[string]struct{}) map[string]ports.GitStatus {
	next := make(map[string]ports.GitStatus, len(current))
	for name, status := range current {
		if _, ok := liveNames[name]; ok {
			next[name] = status
		}
	}
	return next
}

var errMetadataReconcile = errors.New("metadata reconcile requested")
var errMetadataFallback = errors.New("metadata watcher fallback")

func (s *MetadataService) Run(ctx context.Context, cfg ports.ConfigSnapshot) error {
	current := cfg
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		loaded, err := s.loadRunConfig(ctx, current)
		if err != nil {
			return err
		}
		current = loaded
		if !current.MetadataSublineEnabled {
			if err := s.waitMetadataReconcile(ctx); errors.Is(err, errMetadataReconcile) {
				continue
			} else {
				return err
			}
		}
		err = s.runWatchOnce(ctx, current)
		switch {
		case errors.Is(err, errMetadataReconcile):
			continue
		case errors.Is(err, errMetadataFallback):
			if err := s.runFallbackPoll(ctx, current); errors.Is(err, errMetadataReconcile) {
				continue
			} else {
				return err
			}
		default:
			return err
		}
	}
}

func (s *MetadataService) loadRunConfig(ctx context.Context, fallback ports.ConfigSnapshot) (ports.ConfigSnapshot, error) {
	if s.Config == nil {
		return fallback, nil
	}
	cfg, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	return cfg, nil
}

func (s *MetadataService) waitMetadataReconcile(ctx context.Context) error {
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
		return errMetadataReconcile
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
	if s.Store == nil || s.Query == nil || s.Git == nil {
		return nil, errors.New("metadata service missing dependency")
	}
	state, err := s.Store.Load(ctx, "tmux")
	if err != nil {
		return nil, err
	}
	live, err := s.Query.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	livePaths := metadataLiveSessionPaths(ctx, s.Query, live)
	subs := map[string]MetadataRepoSubscription{}
	infos := map[string]metadataRepoInfoResult{}
	targetsByWorktree := map[string]metadataWatchTargetsResult{}
	for _, session := range live {
		path, ok := sessionMetadataCapturePath(session.Name, state.Sessions[session.Name], livePaths)
		if !ok {
			continue
		}
		pathKey := metadataPathCacheKey(path)
		infoResult, ok := infos[pathKey]
		if !ok {
			info, err := s.Git.RepoInfo(ctx, path)
			infoResult = metadataRepoInfoResult{info: info, err: err}
			infos[pathKey] = infoResult
		}
		if infoResult.err != nil {
			if errors.Is(infoResult.err, ports.ErrNotGitRepository) || errors.Is(infoResult.err, ports.ErrGitPathMissing) {
				continue
			}
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata repo info failed for session %q path %q: %v\n", session.Name, path, infoResult.err)
			continue
		}
		info := infoResult.info
		worktreeKey := metadataWorktreeCacheKey(info)
		targetResult, ok := targetsByWorktree[worktreeKey]
		if !ok {
			targets, err := s.Git.WatchTargets(ctx, path)
			targetResult = metadataWatchTargetsResult{targets: targets, err: err}
			targetsByWorktree[worktreeKey] = targetResult
		}
		if targetResult.err != nil {
			if errors.Is(targetResult.err, ports.ErrNotGitRepository) || errors.Is(targetResult.err, ports.ErrGitPathMissing) {
				continue
			}
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata watch targets failed for session %q path %q: %v\n", session.Name, path, targetResult.err)
			continue
		}
		targets := targetResult.targets
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

type metadataRepoInfoResult struct {
	info ports.GitRepoInfo
	err  error
}

type metadataWatchTargetsResult struct {
	targets ports.GitWatchTargets
	err     error
}

func metadataLiveSessionPaths(ctx context.Context, query ports.QueryPort, live []ports.SessionSnapshot) map[string]string {
	if batchQuery, ok := query.(metadataSessionPathsQuery); ok {
		names := make([]string, 0, len(live))
		for _, session := range live {
			names = append(names, session.Name)
		}
		if paths, err := batchQuery.SessionPaths(ctx, names); err == nil {
			return paths
		}
	}
	livePaths := make(map[string]string, len(live))
	for _, session := range live {
		if path, err := query.SessionPath(ctx, session.Name); err == nil {
			livePaths[session.Name] = path
		}
	}
	return livePaths
}

func metadataPathCacheKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func metadataWorktreeCacheKey(info ports.GitRepoInfo) string {
	path := info.WorktreeRoot
	if strings.TrimSpace(path) == "" {
		path = info.RepoRoot
	}
	return metadataPathCacheKey(path)
}

func (s *MetadataService) gitStatusTimeout() time.Duration {
	if s.GitStatusTimeout > 0 {
		return s.GitStatusTimeout
	}
	return metadataGitStatusTimeout
}

func (s *MetadataService) captureFailureCooldown() time.Duration {
	if s.CaptureFailureCooldown != 0 {
		return s.CaptureFailureCooldown
	}
	return defaultMetadataCaptureFailureCooldown
}

func (s *MetadataService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *MetadataService) captureInCooldown(repoRoot string) bool {
	cooldown := s.captureFailureCooldown()
	if cooldown <= 0 {
		return false
	}
	now := s.now()
	s.captureFailureMu.Lock()
	defer s.captureFailureMu.Unlock()
	until, ok := s.captureFailureUntil[repoRoot]
	if !ok {
		return false
	}
	if !now.Before(until) {
		delete(s.captureFailureUntil, repoRoot)
		return false
	}
	return true
}

func (s *MetadataService) recordCaptureFailure(repoRoot string) {
	cooldown := s.captureFailureCooldown()
	if cooldown <= 0 {
		return
	}
	s.captureFailureMu.Lock()
	defer s.captureFailureMu.Unlock()
	if s.captureFailureUntil == nil {
		s.captureFailureUntil = map[string]time.Time{}
	}
	s.captureFailureUntil[repoRoot] = s.now().Add(cooldown)
}

func (s *MetadataService) clearCaptureFailure(repoRoot string) {
	s.captureFailureMu.Lock()
	defer s.captureFailureMu.Unlock()
	delete(s.captureFailureUntil, repoRoot)
}

func (s *MetadataService) CaptureRepo(ctx context.Context, sub MetadataRepoSubscription) (bool, error) {
	if s.Store == nil || s.Git == nil {
		return false, errors.New("metadata service missing dependency")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.captureInCooldown(sub.RepoRoot) {
		return false, nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, s.gitStatusTimeout())
	defer cancel()
	status, err := s.Git.Status(statusCtx, sub.WorktreeRoot)
	terminalDelete := false
	if err != nil {
		if errors.Is(err, ports.ErrNotGitRepository) || errors.Is(err, ports.ErrGitPathMissing) {
			terminalDelete = true
		} else {
			s.recordCaptureFailure(sub.RepoRoot)
			return false, err
		}
	}
	s.clearCaptureFailure(sub.RepoRoot)
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
