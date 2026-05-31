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
