package app

import (
	"context"
	"errors"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type ColorSchemeService struct {
	Source    ports.SystemColorSchemePort
	Tmux      ports.TmuxConfigPort
	Refresher ports.SidebarRefresherPort
}

func NewColorSchemeService() *ColorSchemeService {
	return &ColorSchemeService{
		Source:    runtimeSystemColorScheme(),
		Tmux:      runtimeTmux(),
		Refresher: runtimeTmux(),
	}
}

func (s *ColorSchemeService) Run(ctx context.Context) error {
	if s == nil || s.Source == nil {
		return nil
	}
	changes, errs, err := s.Source.Watch(ctx)
	if err != nil {
		return err
	}
	last, err := s.Source.CurrentPreference(ctx)
	if err != nil {
		last = config.SystemColorSchemeNoPreference
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		case preference, ok := <-changes:
			if !ok {
				// If the context was cancelled (normal shutdown), return that.
				// Otherwise the watcher dropped the connection without sending
				// an error first — treat this as a failure.
				if err := ctx.Err(); err != nil {
					return err
				}
				return errors.New("color scheme watcher closed unexpectedly")
			}
			if preference == last {
				continue
			}
			last = preference
			if s.Tmux == nil || s.Refresher == nil {
				continue
			}
			cfg, err := s.Tmux.LoadConfig(ctx)
			if err != nil {
				continue
			}
			if config.ParseColorSchemeMode(string(cfg.ColorSchemeMode)) != config.ColorSchemeModeSystem {
				continue
			}
			if err := s.Refresher.RefreshAllSidebars(ctx); err != nil && !errors.Is(err, context.Canceled) {
				continue
			}
		}
	}
}
