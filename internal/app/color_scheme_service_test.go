package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestColorSchemeServiceRunRefreshesFollowSystemSidebars(t *testing.T) {
	source := newFakeSystemColorSchemeSource()
	var refreshes atomic.Int64
	svc := ColorSchemeService{
		Source: source,
		Tmux: colorSchemeConfigLoader(func(context.Context) (ports.ConfigSnapshot, error) {
			return ports.ConfigSnapshot{ColorSchemeMode: config.ColorSchemeModeSystem}, nil
		}),
		Refresher: colorSchemeRefresher(func(context.Context) error {
			refreshes.Add(1)
			return nil
		}),
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	source.waitStarted(t)

	source.changes <- config.SystemColorSchemePreferDark
	eventuallyColorScheme(t, func() bool { return refreshes.Load() == 1 })
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

func TestColorSchemeServiceRunIgnoresForcedModes(t *testing.T) {
	tests := map[string]config.ColorSchemeMode{
		"force light": config.ColorSchemeModeLight,
		"force dark":  config.ColorSchemeModeDark,
	}
	for name, mode := range tests {
		t.Run(name, func(t *testing.T) {
			source := newFakeSystemColorSchemeSource()
			var refreshes atomic.Int64
			svc := ColorSchemeService{
				Source: source,
				Tmux: colorSchemeConfigLoader(func(context.Context) (ports.ConfigSnapshot, error) {
					return ports.ConfigSnapshot{ColorSchemeMode: mode}, nil
				}),
				Refresher: colorSchemeRefresher(func(context.Context) error {
					refreshes.Add(1)
					return nil
				}),
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- svc.Run(ctx) }()
			source.waitStarted(t)

			source.changes <- config.SystemColorSchemePreferLight
			time.Sleep(20 * time.Millisecond)
			if got := refreshes.Load(); got != 0 {
				t.Fatalf("refreshes = %d, want 0", got)
			}
			cancel()
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run error: %v", err)
			}
		})
	}
}

type fakeSystemColorSchemeSource struct {
	changes chan config.SystemColorSchemePreference
	errs    chan error
	started chan struct{}
}

func newFakeSystemColorSchemeSource() *fakeSystemColorSchemeSource {
	return &fakeSystemColorSchemeSource{
		changes: make(chan config.SystemColorSchemePreference, 8),
		errs:    make(chan error, 1),
		started: make(chan struct{}),
	}
}

func (f *fakeSystemColorSchemeSource) CurrentPreference(context.Context) (config.SystemColorSchemePreference, error) {
	return config.SystemColorSchemeNoPreference, nil
}

func (f *fakeSystemColorSchemeSource) Watch(context.Context) (<-chan config.SystemColorSchemePreference, <-chan error, error) {
	close(f.started)
	return f.changes, f.errs, nil
}

func (f *fakeSystemColorSchemeSource) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("color scheme watcher did not start")
	}
}

type colorSchemeConfigLoader func(context.Context) (ports.ConfigSnapshot, error)

func (fn colorSchemeConfigLoader) LoadConfig(ctx context.Context) (ports.ConfigSnapshot, error) {
	return fn(ctx)
}

type colorSchemeRefresher func(context.Context) error

func (fn colorSchemeRefresher) RefreshAllSidebars(ctx context.Context) error {
	return fn(ctx)
}

func eventuallyColorScheme(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition was not met before timeout")
		case <-tick.C:
		}
	}
}
