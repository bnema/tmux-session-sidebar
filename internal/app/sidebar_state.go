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
	if !state.Open {
		return false
	}
	owner := strings.TrimSpace(state.OwnerClient)
	client = strings.TrimSpace(client)
	return owner == "" || owner == client
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
		sidebar.Open = open
		if open {
			if trimmed := strings.TrimSpace(client); trimmed != "" {
				sidebar.OwnerClient = trimmed
			}
		} else {
			sidebar.OwnerClient = ""
		}
	})
}
