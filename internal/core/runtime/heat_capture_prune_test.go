package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestCaptureSessionHeatWithConfigPrunesHeatForNonLiveSessions(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	store := mocks.NewMockStateStorePort(t)
	query := runtimeTestQuery{
		live:    []ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		clients: []ports.ClientSnapshot{},
	}

	initial := ports.PersistedState{
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-5 * time.Minute)},
			"stale": {LastActiveAt: now.Add(-time.Hour)},
		}),
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := decodeHeatStateMap(state.Heat)
		if _, ok := decoded["alpha"]; !ok {
			return false
		}
		_, stale := decoded["stale"]
		return !stale
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{HeatColorsEnabled: true}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}

func TestCaptureSessionHeatWithConfigSkipsWhenHeatColorsAndAutoSortDisabled(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
	store.AssertNotCalled(t, "Load")
	store.AssertNotCalled(t, "Save")
	query.AssertNotCalled(t, "ListSessions")
	query.AssertNotCalled(t, "ListClients")
}

func TestCaptureSessionHeatWithConfigCapturesWhenHeatColorsEnabled(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return len(state.Heat) == 1
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{HeatColorsEnabled: true}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}

func TestCaptureSessionHeatWithConfigCapturesWithoutSortingWhenAutoSortRecentEnabledAndNoActivity(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return len(state.Heat) == 1 && state.Sidebar == nil
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{AutoSortRecentInterval: time.Hour}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}
