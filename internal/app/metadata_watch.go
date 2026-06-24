package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type metadataWatchCoordinator struct {
	service *MetadataService
}

func newMetadataWatchCoordinator(service *MetadataService) metadataWatchCoordinator {
	return metadataWatchCoordinator{service: service}
}

func (w metadataWatchCoordinator) runWatchOnce(ctx context.Context, cfg ports.ConfigSnapshot) error {
	s := w.service
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

func (w metadataWatchCoordinator) runFallbackPoll(ctx context.Context, cfg ports.ConfigSnapshot) error {
	s := w.service
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
