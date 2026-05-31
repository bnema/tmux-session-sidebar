package gitgo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Git struct {
	Fallback ports.GitPort
}

func (g Git) RepoRoot(ctx context.Context, path string) (string, error) {
	info, err := g.RepoInfo(ctx, path)
	if err != nil {
		return "", err
	}
	return info.RepoRoot, nil
}

func (g Git) RepoInfo(ctx context.Context, path string) (ports.GitRepoInfo, error) {
	info, err := g.repoInfo(ctx, path)
	if err != nil && g.Fallback != nil && shouldUseFallback(err) {
		return g.Fallback.RepoInfo(ctx, path)
	}
	return info, err
}

func (g Git) repoInfo(ctx context.Context, path string) (ports.GitRepoInfo, error) {
	if err := ctx.Err(); err != nil {
		return ports.GitRepoInfo{}, err
	}
	info, err := discoverRepoInfo(path)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	repo, err := openRepo(info.WorktreeRoot)
	if err != nil {
		return ports.GitRepoInfo{}, mapGoGitError(path, err)
	}
	branch, err := branchName(repo, info)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	info.Branch = branch
	info.DefaultBranch = defaultRemoteBranch(repo)
	return info, ctx.Err()
}

func (g Git) WatchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	info, err := g.watchTargets(ctx, path)
	if err != nil && g.Fallback != nil && shouldUseFallback(err) {
		return g.Fallback.WatchTargets(ctx, path)
	}
	return info, err
}

func (g Git) watchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	info, err := g.repoInfo(ctx, path)
	if err != nil {
		return ports.GitWatchTargets{}, err
	}
	files := []string{
		filepath.Join(info.GitDir, "HEAD"),
		filepath.Join(info.GitDir, "index"),
		filepath.Join(info.CommonGitDir, "packed-refs"),
	}
	dirs := []string{
		info.WorktreeRoot,
		info.GitDir,
		filepath.Join(info.CommonGitDir, "refs"),
	}
	if info.GitDir != info.CommonGitDir {
		dirs = append(dirs, info.CommonGitDir, filepath.Join(info.GitDir, "refs"))
	}
	return ports.GitWatchTargets{
		RepoRoot:     info.RepoRoot,
		WorktreeRoot: info.WorktreeRoot,
		GitDir:       info.GitDir,
		CommonGitDir: info.CommonGitDir,
		Files:        files,
		Dirs:         dirs,
	}, nil
}

func (g Git) Status(ctx context.Context, path string) (ports.GitStatus, error) {
	status, err := g.status(ctx, path)
	if err != nil && g.Fallback != nil && shouldUseFallback(err) {
		return g.Fallback.Status(ctx, path)
	}
	return status, err
}

func (g Git) status(ctx context.Context, path string) (ports.GitStatus, error) {
	if err := ctx.Err(); err != nil {
		return ports.GitStatus{}, err
	}
	info, err := g.RepoInfo(ctx, path)
	if err != nil {
		return ports.GitStatus{}, err
	}
	repo, err := openRepo(info.WorktreeRoot)
	if err != nil {
		return ports.GitStatus{}, mapGoGitError(path, err)
	}
	status := ports.GitStatus{RepoRoot: info.RepoRoot, Branch: info.Branch}
	ahead, behind, ok, err := divergence(ctx, repo, info.Branch, info.DefaultBranch)
	if err != nil {
		return ports.GitStatus{}, err
	}
	if ok {
		status.Ahead = ahead
		status.Behind = behind
		status.UpstreamConfigured = true
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return ports.GitStatus{}, mapGoGitError(path, err)
	}
	wtStatus, err := worktree.StatusWithOptions(gogit.StatusOptions{Strategy: gogit.Preload})
	if err != nil {
		return ports.GitStatus{}, err
	}
	countStatus(wtStatus, mergeInProgress(info.GitDir), &status)
	status.Clean = status.Ahead == 0 && status.Behind == 0 && status.Staged == 0 && status.Modified == 0 && status.Deleted == 0 && status.Renamed == 0 && status.Untracked == 0 && status.Conflicts == 0
	return status, ctx.Err()
}

func openRepo(path string) (*gogit.Repository, error) {
	return gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
}

func discoverRepoInfo(path string) (ports.GitRepoInfo, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	stat, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ports.GitRepoInfo{}, errors.Join(ports.ErrGitPathMissing, err)
		}
		return ports.GitRepoInfo{}, err
	}
	if !stat.IsDir() {
		abs = filepath.Dir(abs)
	}
	resolvedAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	abs = resolvedAbs
	for dir := abs; ; dir = filepath.Dir(dir) {
		gitPath := filepath.Join(dir, ".git")
		gitStat, statErr := os.Stat(gitPath)
		if statErr == nil {
			gitDir := gitPath
			if !gitStat.IsDir() {
				resolved, err := readGitDirFile(gitPath)
				if err != nil {
					return ports.GitRepoInfo{}, err
				}
				gitDir = resolvePath(dir, resolved)
			}
			commonGitDir, err := readCommonGitDir(gitDir)
			if err != nil {
				return ports.GitRepoInfo{}, err
			}
			worktreeRoot := filepath.Clean(dir)
			return ports.GitRepoInfo{RepoRoot: worktreeRoot, WorktreeRoot: worktreeRoot, GitDir: filepath.Clean(gitDir), CommonGitDir: commonGitDir}, nil
		}
		if !os.IsNotExist(statErr) {
			return ports.GitRepoInfo{}, statErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return ports.GitRepoInfo{}, ports.ErrNotGitRepository
}

func readGitDirFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	gitDir, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("parse %s: missing gitdir prefix", path)
	}
	return strings.TrimSpace(gitDir), nil
}

func readCommonGitDir(gitDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(gitDir), nil
		}
		return "", err
	}
	return resolvePath(gitDir, strings.TrimSpace(string(data))), nil
}

func resolvePath(base string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func branchName(repo *gogit.Repository, info ports.GitRepoInfo) (string, error) {
	head, err := repo.Head()
	if err == nil {
		if head.Name().IsBranch() {
			return head.Name().Short(), nil
		}
		if target := head.Target(); target.IsBranch() {
			return target.Short(), nil
		}
		if hash := head.Hash().String(); hash != "0000000000000000000000000000000000000000" {
			return shortHash(hash), nil
		}
	}
	if err != nil {
		if branch, ok := branchFromHeadFile(info.GitDir); ok {
			return branch, nil
		}
	}
	if err != nil && !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", err
	}
	return "detached", nil
}

func branchFromHeadFile(gitDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	ref, ok := strings.CutPrefix(line, "ref: ")
	if !ok {
		return "", false
	}
	name := plumbing.ReferenceName(strings.TrimSpace(ref))
	if !name.IsBranch() {
		return "", false
	}
	return name.Short(), true
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func defaultRemoteBranch(repo *gogit.Repository) string {
	ref, err := repo.Reference(plumbing.NewRemoteHEADReferenceName("origin"), false)
	if err != nil {
		return ""
	}
	if target := ref.Target(); target != "" {
		return target.Short()
	}
	return ref.Name().Short()
}

func upstreamBranch(repo *gogit.Repository, branch string) string {
	if branch == "" || branch == "detached" {
		return ""
	}
	cfg, err := repo.Config()
	if err != nil {
		return ""
	}
	branchCfg, ok := cfg.Branches[branch]
	if !ok || branchCfg.Remote == "" || branchCfg.Merge == "" {
		return ""
	}
	return plumbing.NewRemoteReferenceName(branchCfg.Remote, branchCfg.Merge.Short()).String()
}

func sameDefaultBranch(branch string, defaultRemote string) bool {
	branch = strings.TrimSpace(branch)
	defaultRemote = strings.TrimSpace(defaultRemote)
	if branch == "" || defaultRemote == "" {
		return false
	}
	return branch == defaultRemote || strings.TrimPrefix(defaultRemote, "origin/") == branch
}

func divergence(ctx context.Context, repo *gogit.Repository, branch string, defaultRemote string) (int, int, bool, error) {
	target := ""
	if defaultRemote != "" && !sameDefaultBranch(branch, defaultRemote) {
		target = defaultRemote
	} else {
		target = upstreamBranch(repo, branch)
		if target == "" {
			target = defaultRemote
		}
	}
	if target == "" {
		return 0, 0, false, nil
	}
	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	targetRef, err := repo.Reference(referenceNameForTarget(target), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, false, err
	}
	ahead, behind, err := aheadBehind(ctx, repo, head.Hash(), targetRef.Hash())
	if err != nil {
		return 0, 0, false, err
	}
	return ahead, behind, true, ctx.Err()
}

func referenceNameForTarget(target string) plumbing.ReferenceName {
	if remote, branch, ok := strings.Cut(target, "/"); ok && remote != "refs" {
		return plumbing.NewRemoteReferenceName(remote, branch)
	}
	return plumbing.ReferenceName(target)
}

func aheadBehind(ctx context.Context, repo *gogit.Repository, head plumbing.Hash, target plumbing.Hash) (int, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	headCommit, err := repo.CommitObject(head)
	if err != nil {
		return 0, 0, err
	}
	targetCommit, err := repo.CommitObject(target)
	if err != nil {
		return 0, 0, err
	}
	mergeBases, err := headCommit.MergeBase(targetCommit)
	if err != nil {
		return 0, 0, err
	}
	limits := make(map[plumbing.Hash]struct{}, len(mergeBases))
	for _, commit := range mergeBases {
		limits[commit.Hash] = struct{}{}
	}
	ahead, err := countUntilLimit(ctx, headCommit, limits)
	if err != nil {
		return 0, 0, err
	}
	behind, err := countUntilLimit(ctx, targetCommit, limits)
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

func countUntilLimit(ctx context.Context, start *object.Commit, limits map[plumbing.Hash]struct{}) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	isLimit := object.CommitFilter(func(commit *object.Commit) bool {
		_, ok := limits[commit.Hash]
		return ok
	})
	iter := object.NewFilterCommitIter(start, nil, &isLimit)
	defer iter.Close()
	count := 0
	err := iter.ForEach(func(commit *object.Commit) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := limits[commit.Hash]; ok {
			return nil
		}
		count++
		return nil
	})
	return count, err
}

func mergeInProgress(gitDir string) bool {
	_, err := os.Stat(filepath.Join(gitDir, "MERGE_HEAD"))
	return err == nil
}

func countStatus(status gogit.Status, mergeActive bool, out *ports.GitStatus) {
	for _, file := range status {
		if file.Staging == gogit.Unmodified && file.Worktree == gogit.Unmodified {
			continue
		}
		if file.Staging == gogit.UpdatedButUnmerged || file.Worktree == gogit.UpdatedButUnmerged || mergeActive && file.Staging != gogit.Unmodified && file.Worktree != gogit.Unmodified {
			out.Conflicts++
			continue
		}
		if file.Staging == gogit.Untracked || file.Worktree == gogit.Untracked {
			out.Untracked++
			continue
		}
		countStaging(file.Staging, out)
		countWorktree(file.Worktree, out)
	}
}

func countStaging(code gogit.StatusCode, out *ports.GitStatus) {
	switch code {
	case gogit.Added, gogit.Modified, gogit.Copied:
		out.Staged++
	case gogit.Deleted:
		out.Deleted++
	case gogit.Renamed:
		out.Renamed++
	}
}

func countWorktree(code gogit.StatusCode, out *ports.GitStatus) {
	switch code {
	case gogit.Modified:
		out.Modified++
	case gogit.Deleted:
		out.Deleted++
	}
}

func shouldUseFallback(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ports.ErrGitPathMissing) && !errors.Is(err, ports.ErrNotGitRepository)
}

func mapGoGitError(path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gogit.ErrRepositoryNotExists) {
		if _, statErr := os.Stat(path); statErr != nil && os.IsNotExist(statErr) {
			return errors.Join(ports.ErrGitPathMissing, err)
		}
		return errors.Join(ports.ErrNotGitRepository, err)
	}
	return err
}
