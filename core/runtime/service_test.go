package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestSnapshotLoadsLiveTmuxState(t *testing.T) {
	tests := []struct {
		name     string
		config   ports.ConfigSnapshot
		sessions []ports.TmuxSessionSnapshot
		clients  []ports.TmuxClientSnapshot
	}{
		{
			name:   "single client and session",
			config: ports.ConfigSnapshot{KeyBinding: "b", Width: "20"},
			sessions: []ports.TmuxSessionSnapshot{
				{ID: "$1", Name: "alpha", WindowCount: 2, AttachedCount: 1},
			},
			clients: []ports.TmuxClientSnapshot{
				{ID: "%1", CurrentSessionID: "$1", CurrentWindowID: "@1", CurrentPaneID: "%9", Attached: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			config := mocks.NewMockTmuxConfigPort(t)
			query := mocks.NewMockTmuxQueryPort(t)
			config.EXPECT().LoadConfig(ctx).Return(tt.config, nil)
			query.EXPECT().ListSessions(ctx).Return(tt.sessions, nil)
			query.EXPECT().ListClients(ctx).Return(tt.clients, nil)

			state, err := NewService(config, query, nil, nil).Snapshot(ctx)
			if err != nil {
				t.Fatalf("Snapshot returned error: %v", err)
			}
			if !reflect.DeepEqual(state.Config, tt.config) {
				t.Fatalf("config = %#v, want %#v", state.Config, tt.config)
			}
			if len(state.Sessions) != len(tt.sessions) || len(state.Clients) != len(tt.clients) {
				t.Fatalf("sessions/clients len = %d/%d, want %d/%d", len(state.Sessions), len(state.Clients), len(tt.sessions), len(tt.clients))
			}
		})
	}
}
