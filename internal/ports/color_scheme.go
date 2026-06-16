package ports

import (
	"context"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
)

type SystemColorSchemePort interface {
	CurrentPreference(ctx context.Context) (config.SystemColorSchemePreference, error)
	Watch(ctx context.Context) (<-chan config.SystemColorSchemePreference, <-chan error, error)
}
