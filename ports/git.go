package ports

import (
	"context"
	"errors"
)

var ErrNotGitRepository = errors.New("not a git repository")
var ErrGitPathMissing = errors.New("git path missing")

type GitStatus struct {
	RepoRoot           string
	Branch             string
	Clean              bool
	Ahead              int
	Behind             int
	Staged             int
	Modified           int
	Deleted            int
	Renamed            int
	Untracked          int
	Conflicts          int
	UpstreamConfigured bool
}

type GitPort interface {
	RepoRoot(ctx context.Context, path string) (string, error)
	Status(ctx context.Context, path string) (GitStatus, error)
}
