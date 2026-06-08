package gitcli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Git struct {
	Process ports.ProcessPort
}

var errMissingUpstream = errors.New("missing upstream")

func (g Git) RepoRoot(ctx context.Context, path string) (string, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", path, "rev-parse", "--show-toplevel"})
	if err != nil {
		return "", mapGitError(result, err)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (g Git) RepoInfo(ctx context.Context, path string) (ports.GitRepoInfo, error) {
	repoRoot, err := g.RepoRoot(ctx, path)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	gitDir, err := g.gitDir(ctx, repoRoot)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	commonGitDir, err := g.commonGitDir(ctx, repoRoot)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	branch, err := g.branch(ctx, repoRoot)
	if err != nil {
		return ports.GitRepoInfo{}, err
	}
	defaultBranch, _ := g.defaultRemoteBranch(ctx, repoRoot)
	return ports.GitRepoInfo{
		RepoRoot:      repoRoot,
		WorktreeRoot:  repoRoot,
		GitDir:        gitDir,
		CommonGitDir:  commonGitDir,
		Branch:        branch,
		DefaultBranch: defaultBranch,
	}, nil
}

func (g Git) WatchTargets(ctx context.Context, path string) (ports.GitWatchTargets, error) {
	info, err := g.RepoInfo(ctx, path)
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
	repoRoot, err := g.RepoRoot(ctx, path)
	if err != nil {
		return ports.GitStatus{}, err
	}
	branch, err := g.branch(ctx, repoRoot)
	if err != nil {
		return ports.GitStatus{}, err
	}
	status := ports.GitStatus{RepoRoot: repoRoot, Branch: branch}
	defaultRemote, _ := g.defaultRemoteBranch(ctx, repoRoot)
	ahead, behind, upstreamConfigured, err := g.Divergence(ctx, repoRoot, branch, defaultRemote)
	if err != nil {
		return ports.GitStatus{}, err
	}
	if upstreamConfigured {
		status.Ahead = ahead
		status.Behind = behind
		status.ComparisonConfigured = true
	}
	upstreamAhead, upstreamBehind, upstreamOK, err := g.UpstreamDivergence(ctx, repoRoot)
	if err != nil {
		return ports.GitStatus{}, err
	}
	if upstreamOK {
		status.UpstreamConfigured = true
		if comparesDefaultRemote(branch, defaultRemote) {
			status.UpstreamAhead = upstreamAhead
			status.UpstreamBehind = upstreamBehind
		}
	}
	if err := g.workingTree(ctx, repoRoot, &status); err != nil {
		return ports.GitStatus{}, err
	}
	status.Clean = status.Ahead == 0 && status.Behind == 0 && status.UpstreamAhead == 0 && status.UpstreamBehind == 0 && status.Staged == 0 && status.Modified == 0 && status.Deleted == 0 && status.Renamed == 0 && status.Untracked == 0 && status.Conflicts == 0
	return status, nil
}

func (g Git) gitDir(ctx context.Context, repoRoot string) (string, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "rev-parse", "--git-dir"})
	if err != nil {
		return "", mapGitError(result, err)
	}
	return absoluteGitPath(repoRoot, strings.TrimSpace(result.Stdout)), nil
}

func (g Git) commonGitDir(ctx context.Context, repoRoot string) (string, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "rev-parse", "--git-common-dir"})
	if err != nil {
		return "", mapGitError(result, err)
	}
	return absoluteGitPath(repoRoot, strings.TrimSpace(result.Stdout)), nil
}

func absoluteGitPath(repoRoot string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(repoRoot, path))
}

func (g Git) branch(ctx context.Context, repoRoot string) (string, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "branch", "--show-current"})
	if err == nil {
		if branch := strings.TrimSpace(result.Stdout); branch != "" {
			return branch, nil
		}
	}
	result, err = g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "rev-parse", "--short", "HEAD"})
	if err != nil {
		if isEmptyRepositoryRevisionError(result, err) {
			return "detached", nil
		}
		return "", mapGitError(result, err)
	}
	commit := strings.TrimSpace(result.Stdout)
	if commit == "" {
		return "detached", nil
	}
	return commit, nil
}

func (g Git) defaultRemoteBranch(ctx context.Context, repoRoot string) (string, bool) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"})
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(result.Stdout)
	return branch, branch != ""
}

func sameDefaultBranch(branch string, defaultRemote string) bool {
	branch = strings.TrimSpace(branch)
	defaultRemote = strings.TrimSpace(defaultRemote)
	if branch == "" || defaultRemote == "" {
		return false
	}
	return branch == defaultRemote || strings.TrimPrefix(defaultRemote, "origin/") == branch
}

func comparesDefaultRemote(branch string, defaultRemote string) bool {
	return defaultRemote != "" && !sameDefaultBranch(branch, defaultRemote)
}

func (g Git) Divergence(ctx context.Context, repoRoot string, branch string, defaultRemote string) (int, int, bool, error) {
	target := "@{upstream}"
	fallbackTarget := ""
	if comparesDefaultRemote(branch, defaultRemote) {
		target = defaultRemote
	} else if branch == "detached" {
		return 0, 0, false, nil
	} else if defaultRemote != "" {
		fallbackTarget = defaultRemote
	}
	ahead, behind, ok, err := g.divergenceAgainst(ctx, repoRoot, target)
	if err == nil || ok || !errors.Is(err, errMissingUpstream) {
		return ahead, behind, ok, err
	}
	if fallbackTarget == "" {
		return 0, 0, false, nil
	}
	ahead, behind, ok, err = g.divergenceAgainst(ctx, repoRoot, fallbackTarget)
	if errors.Is(err, errMissingUpstream) {
		return 0, 0, false, nil
	}
	return ahead, behind, ok, err
}

func (g Git) UpstreamDivergence(ctx context.Context, repoRoot string) (int, int, bool, error) {
	ahead, behind, ok, err := g.divergenceAgainst(ctx, repoRoot, "@{upstream}")
	if errors.Is(err, errMissingUpstream) {
		return 0, 0, false, nil
	}
	return ahead, behind, ok, err
}

func (g Git) divergenceAgainst(ctx context.Context, repoRoot string, target string) (int, int, bool, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "rev-list", "--left-right", "--count", "HEAD..." + target})
	if err != nil {
		if isMissingUpstreamError(result, err) {
			return 0, 0, false, errors.Join(errMissingUpstream, err)
		}
		return 0, 0, false, mapGitError(result, err)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 2 {
		return 0, 0, false, fmt.Errorf("git rev-list upstream count output %q", strings.TrimSpace(result.Stdout))
	}
	ahead, errAhead := strconv.Atoi(fields[0])
	behind, errBehind := strconv.Atoi(fields[1])
	if errAhead != nil || errBehind != nil {
		return 0, 0, false, fmt.Errorf("parse git rev-list upstream count %q", strings.TrimSpace(result.Stdout))
	}
	return ahead, behind, true, nil
}

func (g Git) workingTree(ctx context.Context, repoRoot string, status *ports.GitStatus) error {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", repoRoot, "status", "--porcelain=v1", "--branch"})
	if err != nil {
		return mapGitError(result, err)
	}
	for line := range strings.SplitSeq(result.Stdout, "\n") {
		if line == "" || strings.HasPrefix(line, "## ") {
			continue
		}
		countStatusLine(line, status)
	}
	return nil
}

func countStatusLine(line string, status *ports.GitStatus) {
	if strings.HasPrefix(line, "??") {
		status.Untracked++
		return
	}
	if len(line) < 2 {
		return
	}
	x := line[0]
	y := line[1]
	if isConflictStatus(x, y) {
		status.Conflicts++
		return
	}
	if x != ' ' {
		countIndexStatus(x, status)
	}
	if y != ' ' {
		countWorktreeStatus(y, status)
	}
}

func isConflictStatus(x byte, y byte) bool {
	return x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
}

func countIndexStatus(statusByte byte, status *ports.GitStatus) {
	switch statusByte {
	case 'A':
		status.Staged++
	case 'M':
		status.Staged++
	case 'D':
		status.Deleted++
	case 'R':
		status.Renamed++
	case 'C':
		status.Staged++
	}
}

func countWorktreeStatus(statusByte byte, status *ports.GitStatus) {
	switch statusByte {
	case 'M', 'T':
		status.Modified++
	case 'D':
		status.Deleted++
	}
}

func isEmptyRepositoryRevisionError(result ports.Result, err error) bool {
	message := strings.ToLower(result.Stderr + result.Stdout + err.Error())
	return strings.Contains(message, "needed a single revision") || strings.Contains(message, "unknown revision") || strings.Contains(message, "ambiguous argument 'head'")
}

func isMissingUpstreamError(result ports.Result, err error) bool {
	message := strings.ToLower(result.Stderr + result.Stdout + err.Error())
	return strings.Contains(message, "no upstream configured") || strings.Contains(message, "no upstream branch") || strings.Contains(message, "does not point to a branch") || strings.Contains(message, "upstream") && strings.Contains(message, "not found") || strings.Contains(message, "unknown revision") || strings.Contains(message, "ambiguous argument") || strings.Contains(message, "no such branch")
}

func mapGitError(result ports.Result, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(result.Stderr + result.Stdout + err.Error())
	if strings.Contains(message, "cannot change to") || strings.Contains(message, "no such file or directory") {
		return errors.Join(ports.ErrGitPathMissing, err)
	}
	if strings.Contains(message, "not a git repository") || strings.Contains(message, "not in a git directory") {
		return errors.Join(ports.ErrNotGitRepository, err)
	}
	return err
}
