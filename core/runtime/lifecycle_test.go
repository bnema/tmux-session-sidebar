package runtime

import (
	"context"
	"errors"
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
		expect   func(context.Context, *mocks.MockTmuxSidebarPort)
		wantOpen bool
		wantPane bool
	}{
		{
			name:  "open delegates to sidebar port",
			state: State{Config: ports.ConfigSnapshot{Width: "20"}, Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, uiCommand []string) (State, error) {
				return service.OpenSidebar(ctx, state, clientID, uiCommand)
			},
			expect: func(ctx context.Context, sidebar *mocks.MockTmuxSidebarPort) {
				sidebar.EXPECT().OpenSidebar(ctx, "%1", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
			},
			wantOpen: true,
			wantPane: true,
		},
		{
			name:  "close delegates to sidebar port",
			state: State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@1"}}},
			act: func(ctx context.Context, service *Service, state State, clientID string, _ []string) (State, error) {
				return service.CloseSidebar(ctx, state, clientID)
			},
			expect: func(ctx context.Context, sidebar *mocks.MockTmuxSidebarPort) {
				sidebar.EXPECT().CloseSidebarPane(ctx, "%9").Return(nil)
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
			expect: func(ctx context.Context, sidebar *mocks.MockTmuxSidebarPort) {
				sidebar.EXPECT().OpenSidebar(ctx, "%1", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%9", WindowID: "@1"}, nil)
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
			expect: func(ctx context.Context, sidebar *mocks.MockTmuxSidebarPort) {
				sidebar.EXPECT().OpenSidebar(ctx, "%1", []string{"ui", "run"}).Return(ports.PaneRef{PaneID: "%10", WindowID: "@2"}, nil)
			},
			wantOpen: true,
			wantPane: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			sidebar := mocks.NewMockTmuxSidebarPort(t)
			tt.expect(ctx, sidebar)
			got, err := tt.act(ctx, NewService(nil, nil, nil, nil).WithSidebar(sidebar), tt.state, "%1", []string{"ui", "run"})
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

func TestCloseSidebarKeepsStateWhenPortCloseFails(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("close failed")
	state := State{
		Clients:  map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}},
		Sidebars: map[string]sidebar.State{"%1": {Open: true}},
		Panes:    map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@1"}},
	}
	sidebarPort := mocks.NewMockTmuxSidebarPort(t)
	sidebarPort.EXPECT().CloseSidebarPane(ctx, "%9").Return(boom)

	got, err := NewService(nil, nil, nil, nil).WithSidebar(sidebarPort).CloseSidebar(ctx, state, "%1")
	if !errors.Is(err, boom) {
		t.Fatalf("CloseSidebar error = %v, want %v", err, boom)
	}
	if !got.Sidebars["%1"].Open {
		t.Fatal("sidebar state was closed after failed port close")
	}
	if got.Panes["%1"].PaneID != "%9" {
		t.Fatalf("pane state = %#v, want original pane", got.Panes["%1"])
	}
}
