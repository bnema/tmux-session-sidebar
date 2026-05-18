package runtime

import (
	"context"
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/clients"
	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestSidebarLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		act      func(context.Context, *Service, State, string, []string) (State, error)
		expect   func(context.Context, *mocks.MockTmuxControlPort)
		wantOpen bool
		wantPane bool
	}{
		{
			name:  "open saves layout and opens pane",
			state: State{Config: ports.ConfigSnapshot{Width: "20"}, Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, uiCommand []string) (State, error) {
				return service.OpenSidebar(ctx, state, clientID, uiCommand)
			},
			expect: func(ctx context.Context, control *mocks.MockTmuxControlPort) {
				control.EXPECT().SaveWindowLayout(ctx, "@1").Return(nil)
				control.EXPECT().OpenSidebarPane(ctx, "%1", "20", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
			},
			wantOpen: true,
			wantPane: true,
		},
		{
			name:  "close kills pane and restores layout",
			state: State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@1"}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, _ []string) (State, error) {
				return service.CloseSidebar(ctx, state, clientID)
			},
			expect: func(ctx context.Context, control *mocks.MockTmuxControlPort) {
				control.EXPECT().ClosePane(ctx, "%9").Return(nil)
				control.EXPECT().RestoreWindowLayout(ctx, "@1").Return(nil)
			},
			wantOpen: false,
			wantPane: false,
		},
		{
			name:  "toggle closed opens",
			state: State{Config: ports.ConfigSnapshot{Width: "20"}, Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: false}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, uiCommand []string) (State, error) {
				return service.ToggleSidebar(ctx, state, clientID, uiCommand)
			},
			expect: func(ctx context.Context, control *mocks.MockTmuxControlPort) {
				control.EXPECT().SaveWindowLayout(ctx, "@1").Return(nil)
				control.EXPECT().OpenSidebarPane(ctx, "%1", "20", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
			},
			wantOpen: true,
			wantPane: true,
		},
		{
			name:  "follow repairs stale pane",
			state: State{Config: ports.ConfigSnapshot{Width: "20"}, Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@2"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@1"}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, uiCommand []string) (State, error) {
				return service.FollowClient(ctx, state, clientID, uiCommand)
			},
			expect: func(ctx context.Context, control *mocks.MockTmuxControlPort) {
				control.EXPECT().SaveWindowLayout(ctx, "@2").Return(nil)
				control.EXPECT().OpenSidebarPane(ctx, "%1", "20", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@2"}, nil)
			},
			wantOpen: true,
			wantPane: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			control := mocks.NewMockTmuxControlPort(t)
			tt.expect(ctx, control)
			got, err := tt.act(ctx, NewService(nil, nil, control, nil), tt.state, "%1", []string{"ui", "run"})
			if err != nil {
				t.Fatalf("lifecycle error: %v", err)
			}
			if got.Sidebars["%1"].Open != tt.wantOpen {
				t.Fatalf("open = %v, want %v", got.Sidebars["%1"].Open, tt.wantOpen)
			}
			_, hasPane := got.Panes["%1"]
			if hasPane != tt.wantPane {
				t.Fatalf("has pane = %v, want %v", hasPane, tt.wantPane)
			}
		})
	}
}
