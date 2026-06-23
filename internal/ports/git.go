package ports

import (
	"context"
	"errors"
)

var ErrNotGitRepository = errors.New("not a git repository")
var ErrGitPathMissing = errors.New("git path missing")

type GitStatus struct {
	RepoRoot string
	Branch   string
	// Clean is true only when the working tree is clean, the primary comparison has no
	// divergence, no separate upstream divergence is pending, and ComparisonMissing is false.
	Clean bool
	// Ahead and Behind compare HEAD with the primary comparison target. Feature branches
	// compare to the default remote branch when available; default branches compare to
	// their upstream when available, otherwise to the default remote branch when available.
	Ahead  int
	Behind int
	// UpstreamAhead and UpstreamBehind track push/pull divergence from an actual upstream
	// ref when that upstream is separate from the primary default-remote comparison.
	UpstreamAhead  int
	UpstreamBehind int
	Staged         int
	Modified       int
	Deleted        int
	Renamed        int
	Untracked      int
	Conflicts      int
	// ComparisonConfigured is true when the primary comparison target exists and was read.
	ComparisonConfigured bool
	// ComparisonMissing is true when a comparison target was configured or selected but the
	// target ref is missing. Clean must be false while ComparisonMissing is true.
	ComparisonMissing bool
	// UpstreamConfigured is true only when an actual upstream ref exists and was used, or
	// separately checked, for upstream divergence.
	UpstreamConfigured bool
}

type GitRepoInfo struct {
	RepoRoot      string
	WorktreeRoot  string
	GitDir        string
	CommonGitDir  string
	Branch        string
	DefaultBranch string
}

type GitWatchTargets struct {
	RepoRoot     string
	WorktreeRoot string
	GitDir       string
	CommonGitDir string
	Files        []string
	Dirs         []string
}

type GitPort interface {
	RepoRoot(ctx context.Context, path string) (string, error)
	Status(ctx context.Context, path string) (GitStatus, error)
	RepoInfo(ctx context.Context, path string) (GitRepoInfo, error)
	WatchTargets(ctx context.Context, path string) (GitWatchTargets, error)
}
