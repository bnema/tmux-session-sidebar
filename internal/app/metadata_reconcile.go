package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type metadataSubscriptionReconciler struct {
	service *MetadataService
}

func newMetadataSubscriptionReconciler(service *MetadataService) metadataSubscriptionReconciler {
	return metadataSubscriptionReconciler{service: service}
}

func (r metadataSubscriptionReconciler) Reconcile(ctx context.Context, cfg ports.ConfigSnapshot) (map[string]MetadataRepoSubscription, error) {
	s := r.service
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
	livePaths, err := metadataLiveSessionPaths(ctx, s.Query, live)
	if err != nil {
		return nil, err
	}
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
