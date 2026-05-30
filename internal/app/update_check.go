package app

import (
	"context"

	coreversion "github.com/bnema/tmux-session-sidebar/core/version"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func newUpdateAvailableCheck(ctx context.Context, checker ports.ReleaseCheckerPort) func(string) (bool, error) {
	return func(currentVersion string) (bool, error) {
		latestTag, err := checker.LatestReleaseTag(ctx)
		if err != nil {
			return false, err
		}
		return coreversion.UpdateAvailable(currentVersion, latestTag), nil
	}
}
