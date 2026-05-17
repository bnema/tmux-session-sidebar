package runtime

import (
	"context"

	"github.com/bnema/tmux-session-sidebar/core/clients"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func (s *Service) Snapshot(ctx context.Context) (State, error) {
	cfg, err := s.tmuxConfig.LoadConfig(ctx)
	if err != nil {
		return State{}, err
	}
	liveSessions, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		return State{}, err
	}
	liveClients, err := s.tmuxQuery.ListClients(ctx)
	if err != nil {
		return State{}, err
	}

	state := State{Config: cfg, Sessions: map[string]ports.TmuxSessionSnapshot{}, Clients: map[string]clients.State{}, Sidebars: map[string]sidebar.State{}, Heat: map[string]heat.State{}, Panes: map[string]ports.PaneRef{}}
	for _, session := range liveSessions {
		state.Sessions[session.ID] = session
	}
	for _, client := range liveClients {
		state.Clients[client.ID] = clients.State{ID: client.ID, CurrentSessionID: client.CurrentSessionID, CurrentWindowID: client.CurrentWindowID, CurrentPaneID: client.CurrentPaneID, Attached: client.Attached}
	}
	return state, nil
}

func ReconcileSidebarPane(state State, clientID string) (State, bool) {
	client, ok := state.Clients[clientID]
	if !ok {
		return state, false
	}
	logical := state.Sidebars[clientID]
	if !logical.Open {
		return state, false
	}
	pane, hasPane := state.Panes[clientID]
	if hasPane && pane.WindowID == client.CurrentWindowID {
		return state, false
	}
	delete(state.Panes, clientID)
	return state, true
}
