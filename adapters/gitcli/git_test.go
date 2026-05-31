package gitcli

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type recordingProcess struct {
	calls []gitCall
}

type gitCall struct {
	cmd  string
	args []string
}

func (p *recordingProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	p.calls = append(p.calls, gitCall{cmd: cmd, args: append([]string(nil), args...)})
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "feat/ui\n"}, nil
	}
	if len(args) >= 4 && args[2] == "rev-list" {
		return ports.Result{Stdout: "2\t1\n"}, nil
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "A  staged.go\n M modified.go\n D deleted.go\nR  old.go -> new.go\n?? new.go\nUU conflict.go\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

func TestGitStatusCollectsBranchDivergenceAndWorkingTreeCounts(t *testing.T) {
	process := &recordingProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.RepoRoot != "/repo" || status.Branch != "feat/ui" {
		t.Fatalf("Status repo/branch = %#v", status)
	}
	if !status.UpstreamConfigured {
		t.Fatalf("UpstreamConfigured = false, want true")
	}
	if status.Ahead != 2 || status.Behind != 1 || status.Staged != 1 || status.Modified != 1 || status.Deleted != 1 || status.Renamed != 1 || status.Untracked != 1 || status.Conflicts != 1 {
		t.Fatalf("Status counts = %#v", status)
	}
	if status.Clean {
		t.Fatal("Status Clean = true, want false")
	}
}

func TestGitStatusComparesWorkingBranchWithDefaultRemoteBranch(t *testing.T) {
	process := &defaultBranchProcess{branch: "feat/ui", defaultRemote: "origin/main"}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.UpstreamConfigured || status.Ahead != 5 || status.Behind != 2 {
		t.Fatalf("Status divergence = %#v, want 5 ahead 2 behind", status)
	}
	if got := process.revListTarget; got != "HEAD...origin/main" {
		t.Fatalf("rev-list target = %q, want HEAD...origin/main", got)
	}
}

func TestGitStatusComparesDefaultBranchWithUpstreamFallback(t *testing.T) {
	process := &defaultBranchProcess{branch: "main", defaultRemote: "origin/main", upstreamOut: "1\t0\n"}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.UpstreamConfigured || status.Ahead != 1 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want 1 ahead 0 behind", status)
	}
	if got := process.revListTarget; got != "HEAD...@{upstream}" {
		t.Fatalf("rev-list target = %q, want HEAD...@{upstream}", got)
	}
}

type defaultBranchProcess struct {
	branch        string
	defaultRemote string
	upstreamOut   string
	revListTarget string
}

func (p *defaultBranchProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: p.branch + "\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: p.defaultRemote + "\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		p.revListTarget = args[len(args)-1]
		if p.upstreamOut != "" {
			return ports.Result{Stdout: p.upstreamOut}, nil
		}
		return ports.Result{Stdout: "5\t2\n"}, nil
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## " + p.branch + "\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

func TestGitRepoInfoAndWatchTargetsFromCLI(t *testing.T) {
	process := &repoInfoProcess{}
	git := Git{Process: process}
	info, err := git.RepoInfo(t.Context(), "/work/sub")
	if err != nil {
		t.Fatalf("RepoInfo error: %v", err)
	}
	if info.RepoRoot != "/repo" || info.WorktreeRoot != "/repo" || info.GitDir != "/repo/.git" || info.CommonGitDir != "/repo/.git" || info.Branch != "feat/ui" || info.DefaultBranch != "origin/main" {
		t.Fatalf("RepoInfo = %#v", info)
	}
	targets, err := git.WatchTargets(t.Context(), "/work/sub")
	if err != nil {
		t.Fatalf("WatchTargets error: %v", err)
	}
	if targets.RepoRoot != "/repo" || targets.WorktreeRoot != "/repo" || targets.GitDir != "/repo/.git" || targets.CommonGitDir != "/repo/.git" {
		t.Fatalf("WatchTargets roots = %#v", targets)
	}
	wantFiles := map[string]bool{
		"/repo/.git/HEAD":        true,
		"/repo/.git/index":       true,
		"/repo/.git/packed-refs": true,
	}
	for _, file := range targets.Files {
		delete(wantFiles, file)
	}
	if len(wantFiles) != 0 {
		t.Fatalf("WatchTargets files missing %#v from %#v", wantFiles, targets.Files)
	}
	wantDirs := map[string]bool{"/repo": true, "/repo/.git": true, "/repo/.git/refs": true}
	for _, dir := range targets.Dirs {
		delete(wantDirs, dir)
	}
	if len(wantDirs) != 0 {
		t.Fatalf("WatchTargets dirs missing %#v from %#v", wantDirs, targets.Dirs)
	}
}

type repoInfoProcess struct{}

func (p *repoInfoProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" {
		switch args[3] {
		case "--show-toplevel":
			return ports.Result{Stdout: "/repo\n"}, nil
		case "--git-dir":
			return ports.Result{Stdout: ".git\n"}, nil
		case "--git-common-dir":
			return ports.Result{Stdout: ".git\n"}, nil
		}
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "feat/ui\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

func TestGitStatusReportsMissingUpstream(t *testing.T) {
	process := &missingUpstreamProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured {
		t.Fatal("UpstreamConfigured = true, want false")
	}
	if !status.Clean {
		t.Fatalf("Clean = false, want true: %#v", status)
	}
}

type missingUpstreamProcess struct{}

func (p *missingUpstreamProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "work\n"}, nil
	}
	if len(args) >= 4 && args[2] == "rev-list" {
		return ports.Result{Stderr: "fatal: no upstream configured for branch 'work'"}, errors.New("exit status 128")
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## work\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

func TestGitStatusPropagatesDivergenceCancellation(t *testing.T) {
	process := &canceledDivergenceProcess{}
	_, err := (Git{Process: process}).Status(t.Context(), "/work")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Status error = %v, want context.Canceled", err)
	}
}

type canceledDivergenceProcess struct{}

func (p *canceledDivergenceProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "work\n"}, nil
	}
	if len(args) >= 4 && args[2] == "rev-list" {
		return ports.Result{}, context.Canceled
	}
	return ports.Result{}, errors.New("unexpected call")
}

func TestGitStatusReportsNonRepo(t *testing.T) {
	process := gitErrorProcess{stderr: "fatal: not a git repository"}
	_, err := (Git{Process: process}).Status(t.Context(), "/work")
	if !errors.Is(err, ports.ErrNotGitRepository) {
		t.Fatalf("Status error = %v, want ErrNotGitRepository", err)
	}
}

func TestGitStatusReportsMissingPath(t *testing.T) {
	process := gitErrorProcess{stderr: "fatal: cannot change to '/missing': No such file or directory"}
	_, err := (Git{Process: process}).Status(t.Context(), "/missing")
	if !errors.Is(err, ports.ErrGitPathMissing) {
		t.Fatalf("Status error = %v, want ErrGitPathMissing", err)
	}
}

type gitErrorProcess struct{ stderr string }

func (p gitErrorProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	return ports.Result{Stderr: p.stderr}, errors.New("exit status 128")
}

func TestGitStatusDetachedHeadWithoutDefaultRemoteHasNoDivergence(t *testing.T) {
	process := &detachedNoDefaultProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured || status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want none for detached head without default remote", status)
	}
}

type detachedNoDefaultProcess struct{}

func (p *detachedNoDefaultProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" {
		switch args[3] {
		case "--show-toplevel":
			return ports.Result{Stdout: "/repo\n"}, nil
		case "--short":
			return ports.Result{Stdout: "abc1234\n"}, nil
		}
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stderr: "fatal: ref refs/remotes/origin/HEAD is not a symbolic ref"}, errors.New("exit status 128")
	}
	if len(args) >= 4 && args[2] == "rev-list" {
		return ports.Result{Stderr: "fatal: HEAD does not point to a branch"}, errors.New("exit status 128")
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## HEAD (no branch)\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}
