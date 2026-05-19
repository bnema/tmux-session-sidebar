package runtime

import (
	"context"
	"fmt"

	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func (s *Service) ToggleSidebar(ctx context.Context, state State, clientID string, uiCommand []string) (State, error) {
	logical := state.Sidebars[clientID]
	if logical.Open {
		return s.CloseSidebar(ctx, state, clientID)
	}
	return s.OpenSidebar(ctx, state, clientID, uiCommand)
}

func (s *Service) OpenSidebar(ctx context.Context, state State, clientID string, uiCommand []string) (State, error) {
	client, ok := state.Clients[clientID]
	if !ok {
		return state, fmt.Errorf("missing client %s", clientID)
	}
	if s.tmuxSidebar == nil {
		return state, ErrMissingTmuxSidebar
	}
	pane, err := s.tmuxSidebar.OpenSidebar(ctx, client.ID, uiCommand)
	if err != nil {
		return state, err
	}
	ensureSidebarMaps(&state)
	logical := state.Sidebars[clientID]
	logical.Open = true
	logical.Mode = sidebar.ModeBrowse
	state.Sidebars[clientID] = logical
	state.Panes[clientID] = pane
	return state, nil
}

func (s *Service) CloseSidebar(ctx context.Context, state State, clientID string) (State, error) {
	if s.tmuxSidebar == nil {
		return state, ErrMissingTmuxSidebar
	}
	var err error
	if pane, ok := state.Panes[clientID]; ok && pane.PaneID != "" {
		err = s.tmuxSidebar.CloseSidebarPane(ctx, pane.PaneID)
	} else {
		err = s.tmuxSidebar.CloseSidebar(ctx, clientID)
	}
	if err != nil {
		return state, err
	}
	ensureSidebarMaps(&state)
	logical := state.Sidebars[clientID]
	logical.Open = false
	state.Sidebars[clientID] = logical
	delete(state.Panes, clientID)
	return state, err
}

func (s *Service) FollowClient(ctx context.Context, state State, clientID string, uiCommand []string) (State, error) {
	state, needsRepair := ReconcileSidebarPane(state, clientID)
	if !needsRepair {
		return state, nil
	}
	return s.OpenSidebar(ctx, state, clientID, uiCommand)
}

func ensureSidebarMaps(state *State) {
	if state.Sidebars == nil {
		state.Sidebars = map[string]sidebar.State{}
	}
	if state.Panes == nil {
		state.Panes = map[string]ports.PaneRef{}
	}
}
