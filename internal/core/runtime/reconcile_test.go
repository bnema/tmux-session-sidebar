package runtime

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/core/clients"
	"github.com/bnema/tmux-session-sidebar/internal/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestReconcileSidebarPane(t *testing.T) {
	tests := []struct {
		name       string
		state      State
		clientID   string
		wantRepair bool
		wantPane   bool
	}{
		{
			name:     "closed sidebar no repair",
			clientID: "%1",
			state:    State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: false}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@2"}}},
			wantPane: true,
		},
		{
			name:     "pane already in current window",
			clientID: "%1",
			state:    State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@1"}}},
			wantPane: true,
		},
		{
			name:       "pane in old window needs repair",
			clientID:   "%1",
			state:      State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{"%1": {PaneID: "%9", WindowID: "@2"}}},
			wantRepair: true,
			wantPane:   false,
		},
		{
			name:       "missing pane needs repair",
			clientID:   "%1",
			state:      State{Clients: map[string]clients.State{"%1": {ID: "%1", CurrentWindowID: "@1"}}, Sidebars: map[string]sidebar.State{"%1": {Open: true}}, Panes: map[string]ports.PaneRef{}},
			wantRepair: true,
			wantPane:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, repair := ReconcileSidebarPane(tt.state, tt.clientID)
			if repair != tt.wantRepair {
				t.Fatalf("repair = %v, want %v", repair, tt.wantRepair)
			}
			_, hasPane := got.Panes[tt.clientID]
			if hasPane != tt.wantPane {
				t.Fatalf("has pane = %v, want %v", hasPane, tt.wantPane)
			}
		})
	}
}
