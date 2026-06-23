package ports

import (
	"context"
	"errors"
)

var ErrNotGitRepository = errors.New("not a git repository")
var ErrGitPathMissing = errors.New("git path missing")

type GitStatus struct {
	RepoRoot             string
	Branch               string
	Clean                bool
	Ahead                int
	Behind               int
	UpstreamAhead        int
	UpstreamBehind       int
	Staged               int
	Modified             int
	Deleted              int
	Renamed              int
	Untracked            int
	Conflicts            int
	ComparisonConfigured bool
	ComparisonMissing    bool
	UpstreamConfigured   bool
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
