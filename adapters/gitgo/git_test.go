package gitgo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestGitStatusCollectsDirtyCountsWithNativeGoGit(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "tracked.txt", "one\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")
	writeFile(t, repo, "tracked.txt", "two\n")
	writeFile(t, repo, "staged.txt", "staged\n")
	runGit(t, repo, "add", "staged.txt")
	writeFile(t, repo, "untracked.txt", "new\n")

	status, err := (Git{}).Status(t.Context(), filepath.Join(repo, "subdir"))
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.RepoRoot != repo || status.Branch != "main" {
		t.Fatalf("Status repo/branch = %#v, want repo %q branch main", status, repo)
	}
	if status.Staged != 1 || status.Modified != 1 || status.Untracked != 1 || status.Clean {
		t.Fatalf("Status dirty counts = %#v, want staged=1 modified=1 untracked=1 dirty", status)
	}
}

func TestGitStatusUsesInjectedDivergenceCounter(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "tracked.txt", "one\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")

	counter := &recordingDivergenceCounter{ahead: 2, behind: 3, ok: true}
	status, err := (Git{Divergence: counter}).Status(t.Context(), repo)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if counter.calls != 1 || counter.repoRoot != repo || counter.branch != "main" {
		t.Fatalf("Divergence calls = %d repo = %q branch = %q, want one call for %q main", counter.calls, counter.repoRoot, counter.branch, repo)
	}
	if !status.UpstreamConfigured || status.Ahead != 2 || status.Behind != 3 || status.Clean {
		t.Fatalf("Status divergence = %#v, want injected 2 ahead 3 behind and dirty", status)
	}
}

type recordingDivergenceCounter struct {
	calls         int
	repoRoot      string
	branch        string
	defaultRemote string
	ahead         int
	behind        int
	ok            bool
	err           error
}

func (c *recordingDivergenceCounter) Divergence(_ context.Context, repoRoot string, branch string, defaultRemote string) (int, int, bool, error) {
	c.calls++
	c.repoRoot = repoRoot
	c.branch = branch
	c.defaultRemote = defaultRemote
	return c.ahead, c.behind, c.ok, c.err
}

func TestGitStatusComparesWorkingBranchToDefaultRemote(t *testing.T) {
	origin := initBareGitRepo(t)
	work := cloneRepo(t, origin)
	writeFile(t, work, "base.txt", "base\n")
	runGit(t, work, "add", "base.txt")
	runGit(t, work, "commit", "-m", "base")
	runGit(t, work, "push", "-u", "origin", "main")
	runGit(t, work, "remote", "set-head", "origin", "main")
	runGit(t, work, "checkout", "-b", "feature")
	writeFile(t, work, "feature.txt", "feature\n")
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature")

	status, err := (Git{}).Status(t.Context(), work)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.UpstreamConfigured || status.Ahead != 1 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want 1 ahead 0 behind vs origin/main", status)
	}
}

func TestGitStatusSeparatesDefaultBranchDivergenceFromUpstreamPushPull(t *testing.T) {
	origin := initBareGitRepo(t)
	work := cloneRepo(t, origin)
	writeFile(t, work, "base.txt", "base\n")
	runGit(t, work, "add", "base.txt")
	runGit(t, work, "commit", "-m", "base")
	runGit(t, work, "push", "-u", "origin", "main")
	runGit(t, work, "remote", "set-head", "origin", "main")
	runGit(t, work, "checkout", "-b", "feature")
	writeFile(t, work, "feature.txt", "feature\n")
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature")
	runGit(t, work, "push", "-u", "origin", "feature")
	runGit(t, work, "checkout", "main")
	writeFile(t, work, "main.txt", "main\n")
	runGit(t, work, "add", "main.txt")
	runGit(t, work, "commit", "-m", "main")
	runGit(t, work, "push", "origin", "main")
	runGit(t, work, "checkout", "feature")
	writeFile(t, work, "feature-2.txt", "feature 2\n")
	runGit(t, work, "add", "feature-2.txt")
	runGit(t, work, "commit", "-m", "feature 2")

	status, err := (Git{}).Status(t.Context(), work)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.UpstreamConfigured || status.Ahead != 2 || status.Behind != 1 || status.UpstreamAhead != 1 || status.UpstreamBehind != 0 {
		t.Fatalf("Status divergence = %#v, want base 2/1 and upstream 1/0", status)
	}
}

func TestGitRepoInfoAndWatchTargets(t *testing.T) {
	repo := initGitRepo(t)
	info, err := (Git{}).RepoInfo(t.Context(), repo)
	if err != nil {
		t.Fatalf("RepoInfo error: %v", err)
	}
	if info.RepoRoot != repo || info.WorktreeRoot != repo || info.GitDir != filepath.Join(repo, ".git") || info.CommonGitDir != filepath.Join(repo, ".git") || info.Branch != "main" {
		t.Fatalf("RepoInfo = %#v", info)
	}
	targets, err := (Git{}).WatchTargets(t.Context(), repo)
	if err != nil {
		t.Fatalf("WatchTargets error: %v", err)
	}
	if targets.WorktreeRoot != repo || targets.GitDir != filepath.Join(repo, ".git") {
		t.Fatalf("WatchTargets roots = %#v", targets)
	}
	assertContains(t, targets.Files, filepath.Join(repo, ".git", "HEAD"))
	assertContains(t, targets.Files, filepath.Join(repo, ".git", "index"))
	assertContains(t, targets.Files, filepath.Join(repo, ".git", "packed-refs"))
	assertContains(t, targets.Dirs, repo)
	assertContains(t, targets.Dirs, filepath.Join(repo, ".git"))
	assertContains(t, targets.Dirs, filepath.Join(repo, ".git", "refs"))
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	return dir
}

func initBareGitRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", "-b", "main", dir)
	return dir
}

func cloneRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "work")
	runGit(t, "", "clone", origin, dir)
	runGit(t, dir, "switch", "-c", "main")
	return dir
}

func writeFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	if !slices.Contains(values, want) {
		t.Fatalf("%q not found in %#v", want, values)
	}
}

func TestGitStatusReportsCleanRepository(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "tracked.txt", "one\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")

	status, err := (Git{}).Status(t.Context(), repo)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.Clean {
		t.Fatalf("Clean = false, want true: %#v", status)
	}
}

func TestGitStatusCountsMergeConflicts(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "conflict.txt", "base\n")
	runGit(t, repo, "add", "conflict.txt")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "checkout", "-b", "left")
	writeFile(t, repo, "conflict.txt", "left\n")
	runGit(t, repo, "commit", "-am", "left")
	runGit(t, repo, "checkout", "main")
	writeFile(t, repo, "conflict.txt", "right\n")
	runGit(t, repo, "commit", "-am", "right")
	runGitAllowFailure(t, repo, "merge", "left")

	status, err := (Git{}).Status(t.Context(), repo)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1: %#v", status.Conflicts, status)
	}
}

func TestGitStatusSupportsLinkedWorktree(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "tracked.txt", "one\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")
	worktree := filepath.Join(t.TempDir(), "feature-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature", worktree)
	writeFile(t, worktree, "tracked.txt", "two\n")

	status, err := (Git{}).Status(t.Context(), worktree)
	if err != nil {
		t.Fatalf("Status linked worktree error: %v", err)
	}
	if status.RepoRoot != worktree || status.Branch != "feature" || status.Modified != 1 {
		t.Fatalf("Linked worktree status = %#v, want feature modified", status)
	}
}

func runGitAllowFailure(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	_ = cmd.Run()
}

func TestGitStatusDoesNotGuessDefaultBranchWithoutOriginHead(t *testing.T) {
	origin := initBareGitRepo(t)
	work := cloneRepo(t, origin)
	writeFile(t, work, "base.txt", "base\n")
	runGit(t, work, "add", "base.txt")
	runGit(t, work, "commit", "-m", "base")
	runGit(t, work, "push", "-u", "origin", "main")
	runGit(t, work, "checkout", "-b", "feature")
	writeFile(t, work, "feature.txt", "feature\n")
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature")

	status, err := (Git{}).Status(t.Context(), work)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured || status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want no divergence without origin/HEAD or upstream", status)
	}
}

func TestGitStatusResolvesSymlinkedRepoPath(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "tracked.txt", "one\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatalf("symlink repo: %v", err)
	}
	writeFile(t, repo, "tracked.txt", "two\n")

	status, err := (Git{}).Status(t.Context(), link)
	if err != nil {
		t.Fatalf("Status symlink error: %v", err)
	}
	if status.RepoRoot != repo || status.Modified != 1 {
		t.Fatalf("Status symlink = %#v, want physical repo modified", status)
	}
}

func TestAheadBehindHonorsCanceledContext(t *testing.T) {
	repoPath := initGitRepo(t)
	writeFile(t, repoPath, "file.txt", "one\n")
	runGit(t, repoPath, "add", "file.txt")
	runGit(t, repoPath, "commit", "-m", "one")
	repo, err := openRepo(repoPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err = aheadBehind(ctx, repo, headRef.Hash(), headRef.Hash())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("aheadBehind error = %v, want context.Canceled", err)
	}
}

func TestAheadBehindStopsAtMergeBase(t *testing.T) {
	repoPath := initGitRepo(t)
	writeFile(t, repoPath, "base.txt", "base\n")
	runGit(t, repoPath, "add", "base.txt")
	runGit(t, repoPath, "commit", "-m", "base")
	runGit(t, repoPath, "checkout", "-b", "feature")
	writeFile(t, repoPath, "feature.txt", "feature\n")
	runGit(t, repoPath, "add", "feature.txt")
	runGit(t, repoPath, "commit", "-m", "feature")
	featureHash := gitHash(t, repoPath, "HEAD")
	runGit(t, repoPath, "checkout", "main")
	writeFile(t, repoPath, "main.txt", "main\n")
	runGit(t, repoPath, "add", "main.txt")
	runGit(t, repoPath, "commit", "-m", "main")
	mainHash := gitHash(t, repoPath, "HEAD")
	repo, err := openRepo(repoPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}

	ahead, behind, err := aheadBehind(t.Context(), repo, plumbing.NewHash(featureHash), plumbing.NewHash(mainHash))
	if err != nil {
		t.Fatalf("aheadBehind error: %v", err)
	}
	if ahead != 1 || behind != 1 {
		t.Fatalf("aheadBehind = %d, %d; want 1, 1", ahead, behind)
	}
}

func gitHash(t *testing.T, dir string, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s failed: %v\n%s", rev, err, out)
	}
	return string(bytes.TrimSpace(out))
}
