package gitcli

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
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
	for _, call := range process.calls {
		if len(call.args) >= 4 && call.args[2] == "status" && slices.Contains(call.args, "--branch") {
			t.Fatalf("status command args = %#v, want porcelain status without unused --branch header", call.args)
		}
	}
}

func TestGitStatusComparesWorkingBranchWithDefaultRemoteBranch(t *testing.T) {
	process := &workingBranchWithoutUpstreamProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.ComparisonConfigured || status.UpstreamConfigured || status.Ahead != 5 || status.Behind != 2 {
		t.Fatalf("Status divergence = %#v, want base comparison 5/2 without upstream", status)
	}
	if got := process.revListTargets[0]; got != "HEAD...origin/main" {
		t.Fatalf("first rev-list target = %q, want HEAD...origin/main", got)
	}
}

func TestGitStatusSeparatesDefaultBranchDivergenceFromUpstreamPushPull(t *testing.T) {
	process := &workingBranchWithUpstreamProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.ComparisonConfigured || !status.UpstreamConfigured || status.Ahead != 5 || status.Behind != 2 || status.UpstreamAhead != 1 || status.UpstreamBehind != 3 {
		t.Fatalf("Status divergence = %#v, want base 5/2 and upstream 1/3", status)
	}
	wantTargets := []string{"HEAD...origin/main", "HEAD...@{upstream}"}
	if !slices.Equal(process.revListTargets, wantTargets) {
		t.Fatalf("rev-list targets = %#v, want %#v", process.revListTargets, wantTargets)
	}
}

func TestGitStatusComparesDefaultBranchWithUpstreamFallback(t *testing.T) {
	process := &defaultBranchProcess{branch: "main", defaultRemote: "origin/main", upstreamOut: "1\t0\n"}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.ComparisonConfigured || !status.UpstreamConfigured || status.Ahead != 1 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want upstream comparison 1/0", status)
	}
	wantTargets := []string{"HEAD...@{upstream}"}
	if !slices.Equal(process.revListTargets, wantTargets) {
		t.Fatalf("rev-list targets = %#v, want %#v", process.revListTargets, wantTargets)
	}
}

func TestGitStatusComparesDefaultBranchWithDefaultRemoteWhenUpstreamMissing(t *testing.T) {
	process := &defaultBranchMissingUpstreamProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.ComparisonConfigured || status.UpstreamConfigured || status.Ahead != 4 || status.Behind != 1 {
		t.Fatalf("Status divergence = %#v, want fallback comparison 4/1 without real upstream", status)
	}
	wantTargets := []string{"HEAD...@{upstream}", "HEAD...origin/main"}
	if !slices.Equal(process.revListTargets, wantTargets) {
		t.Fatalf("rev-list targets = %#v, want %#v", process.revListTargets, wantTargets)
	}
}

func TestGitStatusReportsStaleDefaultRemoteBranch(t *testing.T) {
	process := &staleDefaultRemoteProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured || status.ComparisonConfigured || !status.ComparisonMissing || status.Ahead != 0 || status.Behind != 0 || status.Clean {
		t.Fatalf("Status divergence = %#v, want explicit missing comparison when origin/main is missing", status)
	}
	wantTargets := []string{"HEAD...@{upstream}", "HEAD...origin/main"}
	if !slices.Equal(process.revListTargets, wantTargets) {
		t.Fatalf("rev-list targets = %#v, want %#v", process.revListTargets, wantTargets)
	}
}

func TestGitStatusReportsStaleUpstreamBranch(t *testing.T) {
	process := &staleUpstreamProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured || status.ComparisonConfigured || !status.ComparisonMissing || status.Ahead != 0 || status.Behind != 0 || status.Clean {
		t.Fatalf("Status divergence = %#v, want explicit missing comparison when tracked origin/main is missing", status)
	}
	wantTargets := []string{"HEAD...@{upstream}", "HEAD...origin/main"}
	if !slices.Equal(process.revListTargets, wantTargets) {
		t.Fatalf("rev-list targets = %#v, want %#v", process.revListTargets, wantTargets)
	}
}

type staleUpstreamProcess struct {
	revListTargets []string
}

func (p *staleUpstreamProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		target := args[len(args)-1]
		p.revListTargets = append(p.revListTargets, target)
		return ports.Result{Stderr: "fatal: no such branch: 'HEAD...'"}, errors.New("exit status 128")
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## main\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

type staleDefaultRemoteProcess struct {
	revListTargets []string
}

func (p *staleDefaultRemoteProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		target := args[len(args)-1]
		p.revListTargets = append(p.revListTargets, target)
		if target == "HEAD...@{upstream}" {
			return ports.Result{Stderr: "fatal: no upstream configured for branch 'main'"}, errors.New("exit status 128")
		}
		if target == "HEAD...origin/main" {
			return ports.Result{Stderr: "fatal: ambiguous argument 'HEAD...origin/main': unknown revision or path not in the working tree."}, errors.New("exit status 128")
		}
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## main\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

type defaultBranchMissingUpstreamProcess struct {
	revListTargets []string
}

func (p *defaultBranchMissingUpstreamProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		target := args[len(args)-1]
		p.revListTargets = append(p.revListTargets, target)
		if target == "HEAD...@{upstream}" {
			return ports.Result{Stderr: "fatal: no upstream configured for branch 'main'"}, errors.New("exit status 128")
		}
		if target == "HEAD...origin/main" {
			return ports.Result{Stdout: "4\t1\n"}, nil
		}
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## main\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

type workingBranchWithoutUpstreamProcess struct {
	revListTargets []string
}

func (p *workingBranchWithoutUpstreamProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "feat/ui\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		target := args[len(args)-1]
		p.revListTargets = append(p.revListTargets, target)
		switch target {
		case "HEAD...origin/main":
			return ports.Result{Stdout: "5\t2\n"}, nil
		case "HEAD...@{upstream}":
			return ports.Result{Stderr: "fatal: no upstream configured for branch 'feat/ui'"}, errors.New("exit status 128")
		}
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## feat/ui\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

type workingBranchWithUpstreamProcess struct {
	revListTargets []string
}

func (p *workingBranchWithUpstreamProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "feat/ui\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stdout: "origin/main\n"}, nil
	}
	if len(args) >= 5 && args[2] == "rev-list" {
		target := args[len(args)-1]
		p.revListTargets = append(p.revListTargets, target)
		switch target {
		case "HEAD...origin/main":
			return ports.Result{Stdout: "5\t2\n"}, nil
		case "HEAD...@{upstream}":
			return ports.Result{Stdout: "1\t3\n"}, nil
		}
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: "## feat/ui\n"}, nil
	}
	return ports.Result{}, errors.New("unexpected call")
}

type defaultBranchProcess struct {
	branch         string
	defaultRemote  string
	upstreamOut    string
	revListTargets []string
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
		p.revListTargets = append(p.revListTargets, args[len(args)-1])
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
	if status.UpstreamConfigured || status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("Status divergence = %#v, want no upstream and zero divergence", status)
	}
	if !status.Clean {
		t.Fatalf("Clean = false, want true: %#v", status)
	}
}

func TestGitStatusReportsStaleConfiguredUpstreamWithoutDefaultRemote(t *testing.T) {
	process := &staleConfiguredUpstreamWithoutDefaultRemoteProcess{}
	status, err := (Git{Process: process}).Status(t.Context(), "/work")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.UpstreamConfigured || status.ComparisonConfigured || !status.ComparisonMissing || status.Ahead != 0 || status.Behind != 0 || status.Clean {
		t.Fatalf("Status divergence = %#v, want missing comparison for stale configured upstream", status)
	}
}

type missingUpstreamProcess struct{}

type staleConfiguredUpstreamWithoutDefaultRemoteProcess struct{}

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

func (p *staleConfiguredUpstreamWithoutDefaultRemoteProcess) Exec(ctx context.Context, cmd string, args []string) (ports.Result, error) {
	if len(args) >= 4 && args[2] == "rev-parse" && args[3] == "--show-toplevel" {
		return ports.Result{Stdout: "/repo\n"}, nil
	}
	if len(args) >= 4 && args[2] == "branch" && args[3] == "--show-current" {
		return ports.Result{Stdout: "work\n"}, nil
	}
	if len(args) >= 5 && args[2] == "symbolic-ref" && args[3] == "--short" && args[4] == "refs/remotes/origin/HEAD" {
		return ports.Result{Stderr: "fatal: ref refs/remotes/origin/HEAD is not a symbolic ref"}, errors.New("exit status 1")
	}
	if len(args) >= 4 && args[2] == "rev-list" {
		return ports.Result{Stderr: "fatal: upstream branch 'origin/work' not found"}, errors.New("exit status 128")
	}
	if len(args) >= 4 && args[2] == "status" {
		return ports.Result{Stdout: ""}, nil
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
	if status.UpstreamConfigured || status.ComparisonConfigured || status.ComparisonMissing || status.Ahead != 0 || status.Behind != 0 || !status.Clean {
		t.Fatalf("Status divergence = %#v, want clean detached head without comparison", status)
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
