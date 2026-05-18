package ports

import "context"

type GitPort interface {
	RepoRoot(ctx context.Context, path string) (string, error)
}
