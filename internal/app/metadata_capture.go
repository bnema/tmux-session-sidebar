package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type metadataCaptureEngine struct {
	service *MetadataService
}

func newMetadataCaptureEngine(service *MetadataService) metadataCaptureEngine {
	return metadataCaptureEngine{service: service}
}

func (e metadataCaptureEngine) Capture(ctx context.Context, cfg ports.ConfigSnapshot, onChange func() error) (changed bool, err error) {
	s := e.service
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

	livePaths, err := metadataLiveSessionPaths(ctx, s.Query, live)
	if err != nil {
		return false, err
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
