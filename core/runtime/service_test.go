package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestSnapshotLoadsLiveTmuxState(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name       string
		config     ports.ConfigSnapshot
		sessions   []ports.TmuxSessionSnapshot
		clients    []ports.TmuxClientSnapshot
		configErr  error
		sessionErr error
		clientErr  error
		wantErr    bool
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
		{name: "config error", configErr: boom, wantErr: true},
		{name: "sessions error", sessionErr: boom, wantErr: true},
		{name: "clients error", clientErr: boom, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			config := mocks.NewMockTmuxConfigPort(t)
			query := mocks.NewMockTmuxQueryPort(t)
			config.EXPECT().LoadConfig(ctx).Return(tt.config, tt.configErr)
			if tt.configErr == nil {
				query.EXPECT().ListSessions(ctx).Return(tt.sessions, tt.sessionErr)
			}
			if tt.configErr == nil && tt.sessionErr == nil {
				query.EXPECT().ListClients(ctx).Return(tt.clients, tt.clientErr)
			}

			state, err := NewService(config, query, nil, nil).Snapshot(ctx)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Snapshot error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(state.Config, tt.config) {
				t.Fatalf("config = %#v, want %#v", state.Config, tt.config)
			}
			if state.Sessions["$1"].Name != "alpha" {
				t.Fatalf("session $1 = %#v", state.Sessions["$1"])
			}
			if state.Clients["%1"].CurrentSessionID != "$1" {
				t.Fatalf("client %%1 = %#v", state.Clients["%1"])
			}
		})
	}
}
