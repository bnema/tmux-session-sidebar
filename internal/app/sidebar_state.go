package app

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func persistedSidebarState(ctx context.Context) (ports.SidebarState, error) {
	state, err := loadSidebarState(ctx)
	if err != nil {
		return ports.SidebarState{}, err
	}
	if state.Sidebar == nil {
		return ports.SidebarState{}, nil
	}
	return *state.Sidebar, nil
}

func ensurePersistedSidebarState(state *ports.PersistedState) *ports.SidebarState {
	if state.Sidebar == nil {
		state.Sidebar = &ports.SidebarState{}
	}
	return state.Sidebar
}

func sidebarStateAppliesToClient(state ports.SidebarState, client string) bool {
	client = strings.TrimSpace(client)
	if state.VisibleClients != nil {
		return client != "" && state.VisibleClients[client]
	}
	if !state.Open {
		return false
	}
	owner := strings.TrimSpace(state.OwnerClient)
	if owner == "" {
		return client == ""
	}
	return owner == client
}

func sidebarShouldBeVisibleForClient(ctx context.Context, client string) (bool, error) {
	state, err := persistedSidebarState(ctx)
	if err != nil {
		return false, err
	}
	return sidebarStateAppliesToClient(state, client), nil
}

func saveSidebarVisibility(ctx context.Context, open bool, client string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		sidebar := ensurePersistedSidebarState(state)
		client = strings.TrimSpace(client)
		if sidebar.VisibleClients == nil {
			sidebar.VisibleClients = visibleClientsFromLegacySidebarState(*sidebar)
		}
		if open {
			if client != "" {
				sidebar.VisibleClients[client] = true
				sidebar.OwnerClient = client
			}
		} else if client != "" {
			delete(sidebar.VisibleClients, client)
			if strings.TrimSpace(sidebar.OwnerClient) == client {
				sidebar.OwnerClient = firstVisibleClient(sidebar.VisibleClients)
			}
		} else {
			sidebar.VisibleClients = map[string]bool{}
			sidebar.OwnerClient = ""
		}
		sidebar.Open = anyVisibleClient(sidebar.VisibleClients)
	})
}

func visibleClientsFromLegacySidebarState(state ports.SidebarState) map[string]bool {
	visible := map[string]bool{}
	if !state.Open {
		return visible
	}
	owner := strings.TrimSpace(state.OwnerClient)
	if owner != "" {
		visible[owner] = true
	}
	return visible
}

func firstVisibleClient(visible map[string]bool) string {
	for client, isVisible := range visible {
		if isVisible {
			return client
		}
	}
	return ""
}

func anyVisibleClient(visible map[string]bool) bool {
	for _, isVisible := range visible {
		if isVisible {
			return true
		}
	}
	return false
}
