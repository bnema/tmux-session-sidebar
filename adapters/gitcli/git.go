package gitcli

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Git struct {
	Process ports.ProcessPort
}

func (g Git) RepoRoot(ctx context.Context, path string) (string, error) {
	result, err := g.Process.Exec(ctx, "git", []string{"-C", path, "rev-parse", "--show-toplevel"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}
