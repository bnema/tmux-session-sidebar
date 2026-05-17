package runtime

import (
	"context"

	"github.com/bnema/tmux-session-sidebar/core/clients"
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

	state := NewState()
	state.Config = cfg
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
	state.Panes = clonePanes(state.Panes)
	delete(state.Panes, clientID)
	return state, true
}

func clonePanes(panes map[string]ports.PaneRef) map[string]ports.PaneRef {
	cloned := make(map[string]ports.PaneRef, len(panes))
	for key, value := range panes {
		cloned[key] = value
	}
	return cloned
}
