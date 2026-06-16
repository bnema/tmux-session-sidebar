package app

import (
	"context"
	"errors"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/ports"
)

const colorSchemeWatchRetryDelay = 200 * time.Millisecond

type ColorSchemeService struct {
	Source    ports.SystemColorSchemePort
	Config    ports.ConfigPort
	Refresher ports.SidebarRefresherPort
}

func NewColorSchemeService() *ColorSchemeService {
	return &ColorSchemeService{
		Source:    runtimeSystemColorScheme(),
		Config:    runtimeMultiplexer(),
		Refresher: runtimeMultiplexer(),
	}
}

func (s *ColorSchemeService) Run(ctx context.Context) error {
	if s == nil || s.Source == nil {
		return nil
	}
	for {
		err := s.runWatchCycle(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}
		if err := waitColorSchemeWatchRetry(ctx); err != nil {
			return err
		}
	}
}

func (s *ColorSchemeService) runWatchCycle(ctx context.Context) error {
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
			if s.Config == nil || s.Refresher == nil {
				continue
			}
			cfg, err := s.Config.LoadConfig(ctx)
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

func waitColorSchemeWatchRetry(ctx context.Context) error {
	timer := time.NewTimer(colorSchemeWatchRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
