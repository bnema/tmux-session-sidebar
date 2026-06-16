package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestColorSchemeServiceRunRefreshesFollowSystemSidebars(t *testing.T) {
	source := mocks.NewMockSystemColorSchemePort(t)
	tmux := mocks.NewMockConfigPort(t)
	refresher := mocks.NewMockSidebarRefresherPort(t)
	changes := make(chan config.SystemColorSchemePreference, 1)
	errs := make(chan error)
	watchStarted := make(chan struct{}, 1)
	var refreshes atomic.Int64

	source.EXPECT().Watch(mock.Anything).Run(func(context.Context) {
		watchStarted <- struct{}{}
	}).Return((<-chan config.SystemColorSchemePreference)(changes), (<-chan error)(errs), nil).Once()
	source.EXPECT().CurrentPreference(mock.Anything).Return(config.SystemColorSchemeNoPreference, nil).Once()
	tmux.EXPECT().LoadConfig(mock.Anything).Return(ports.ConfigSnapshot{ColorSchemeMode: config.ColorSchemeModeSystem}, nil).Once()
	refresher.EXPECT().RefreshAllSidebars(mock.Anything).Run(func(context.Context) {
		refreshes.Add(1)
	}).Return(nil).Once()

	svc := ColorSchemeService{Source: source, Config: tmux, Refresher: refresher}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	waitColorSchemeWatchStarted(t, watchStarted)

	changes <- config.SystemColorSchemePreferDark
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
			source := mocks.NewMockSystemColorSchemePort(t)
			tmux := mocks.NewMockConfigPort(t)
			refresher := mocks.NewMockSidebarRefresherPort(t)
			changes := make(chan config.SystemColorSchemePreference, 1)
			errs := make(chan error)
			watchStarted := make(chan struct{}, 1)
			configLoaded := make(chan struct{}, 1)
			var refreshes atomic.Int64

			source.EXPECT().Watch(mock.Anything).Run(func(context.Context) {
				watchStarted <- struct{}{}
			}).Return((<-chan config.SystemColorSchemePreference)(changes), (<-chan error)(errs), nil).Once()
			source.EXPECT().CurrentPreference(mock.Anything).Return(config.SystemColorSchemeNoPreference, nil).Once()
			tmux.EXPECT().LoadConfig(mock.Anything).Run(func(context.Context) {
				configLoaded <- struct{}{}
			}).Return(ports.ConfigSnapshot{ColorSchemeMode: mode}, nil).Once()
			svc := ColorSchemeService{Source: source, Config: tmux, Refresher: refresher}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- svc.Run(ctx) }()
			waitColorSchemeWatchStarted(t, watchStarted)

			changes <- config.SystemColorSchemePreferLight
			select {
			case <-configLoaded:
			case <-time.After(time.Second):
				t.Fatal("service did not load config for forced mode change")
			}
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

func TestColorSchemeServiceRunRestartsWatcherAfterSourceError(t *testing.T) {
	source := mocks.NewMockSystemColorSchemePort(t)
	tmux := mocks.NewMockConfigPort(t)
	refresher := mocks.NewMockSidebarRefresherPort(t)
	changes1 := make(chan config.SystemColorSchemePreference, 1)
	errs1 := make(chan error, 1)
	changes2 := make(chan config.SystemColorSchemePreference, 1)
	errs2 := make(chan error)
	watchCalls := make(chan int, 2)
	var refreshes atomic.Int64

	firstWatch := source.EXPECT().Watch(mock.Anything).Run(func(context.Context) {
		watchCalls <- 1
	}).Return((<-chan config.SystemColorSchemePreference)(changes1), (<-chan error)(errs1), nil)
	firstWatch.Once()
	firstCurrent := source.EXPECT().CurrentPreference(mock.Anything).Return(config.SystemColorSchemeNoPreference, nil)
	firstCurrent.Once()
	secondWatch := source.EXPECT().Watch(mock.Anything).Run(func(context.Context) {
		watchCalls <- 2
	}).Return((<-chan config.SystemColorSchemePreference)(changes2), (<-chan error)(errs2), nil)
	secondWatch.Once()
	secondCurrent := source.EXPECT().CurrentPreference(mock.Anything).Return(config.SystemColorSchemeNoPreference, nil)
	secondCurrent.Once()
	mock.InOrder(firstWatch.Call, firstCurrent.Call, secondWatch.Call, secondCurrent.Call)
	tmux.EXPECT().LoadConfig(mock.Anything).Return(ports.ConfigSnapshot{ColorSchemeMode: config.ColorSchemeModeSystem}, nil).Once()
	refresher.EXPECT().RefreshAllSidebars(mock.Anything).Run(func(context.Context) {
		refreshes.Add(1)
	}).Return(nil).Once()

	svc := ColorSchemeService{Source: source, Config: tmux, Refresher: refresher}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	waitColorSchemeWatchCall(t, watchCalls, 1)

	errs1 <- errors.New("watch failed")
	waitColorSchemeWatchCall(t, watchCalls, 2)
	changes2 <- config.SystemColorSchemePreferDark
	eventuallyColorScheme(t, func() bool { return refreshes.Load() == 1 })

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

func waitColorSchemeWatchStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("color scheme watcher did not start")
	}
}

func waitColorSchemeWatchCall(t *testing.T, calls <-chan int, want int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("watch call = %d, want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("watch call %d did not start", want)
	}
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
