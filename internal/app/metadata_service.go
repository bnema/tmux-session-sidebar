package app

import (
	"context"
	"errors"
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

func (s *MetadataService) capture(ctx context.Context, cfg ports.ConfigSnapshot, onChange func() error) (bool, error) {
	return newMetadataCaptureEngine(s).Capture(ctx, cfg, onChange)
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
		loaded, err := s.loadRunConfig(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
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

func (s *MetadataService) loadRunConfig(ctx context.Context) (ports.ConfigSnapshot, error) {
	if s.Config == nil {
		return ports.ConfigSnapshot{}, errors.New("metadata service missing config dependency")
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
	return newMetadataWatchCoordinator(s).runWatchOnce(ctx, cfg)
}

func (s *MetadataService) runFallbackPoll(ctx context.Context, cfg ports.ConfigSnapshot) error {
	return newMetadataWatchCoordinator(s).runFallbackPoll(ctx, cfg)
}

func (s *MetadataService) Reconcile(ctx context.Context, cfg ports.ConfigSnapshot) (map[string]MetadataRepoSubscription, error) {
	return newMetadataSubscriptionReconciler(s).Reconcile(ctx, cfg)
}

type metadataRepoInfoResult struct {
	info ports.GitRepoInfo
	err  error
}

type metadataWatchTargetsResult struct {
	targets ports.GitWatchTargets
	err     error
}

func metadataLiveSessionPaths(ctx context.Context, query ports.QueryPort, live []ports.SessionSnapshot) (map[string]string, error) {
	names := make([]string, 0, len(live))
	for _, session := range live {
		if session.Name != "" {
			names = append(names, session.Name)
		}
	}
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	paths, err := query.SessionPaths(ctx, names)
	if err != nil {
		return nil, err
	}
	return paths, nil
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
