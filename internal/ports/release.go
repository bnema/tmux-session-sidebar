package ports

import "context"

type ReleaseCheckerPort interface {
	LatestReleaseTag(ctx context.Context) (string, error)
}
