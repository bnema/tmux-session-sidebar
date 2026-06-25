package ports

import (
	"context"
	"maps"
	"reflect"
)

type PersistedState struct {
	Sessions       map[string]SessionMetadata `json:"sessions,omitempty"`
	SessionOrder   []string                   `json:"sessionOrder,omitempty"`
	PinnedSessions []string                   `json:"pinnedSessions,omitempty"`
	PinColors      map[string]string          `json:"pinColors,omitempty"`
	Sidebar        *SidebarState              `json:"sidebar,omitempty"`
	SidebarLayout  *SidebarLayout             `json:"sidebarLayout,omitempty"`
	Clients        map[string][]byte          `json:"clients,omitempty"`
	Heat           map[string][]byte          `json:"heat,omitempty"`
	AgentAttention map[string][]byte          `json:"agentAttention,omitempty"`
	Metadata       map[string]GitStatus       `json:"metadata,omitempty"`
}

type SidebarLayout struct {
	Items []SidebarLayoutItem `json:"items,omitempty"`
}

type SidebarLayoutItem struct {
	ID        string                 `json:"id,omitempty"`
	Kind      string                 `json:"kind,omitempty"`
	Category  *SidebarLayoutCategory `json:"category,omitempty"`
	Separator *SidebarLayoutSpacer   `json:"separator,omitempty"`
	Spacer    *SidebarLayoutSpacer   `json:"spacer,omitempty"`
}

type SidebarLayoutCategory struct {
	ID               string                    `json:"id,omitempty"`
	Name             string                    `json:"name,omitempty"`
	Color            string                    `json:"color,omitempty"`
	Collapsed        bool                      `json:"collapsed,omitempty"`
	SessionsExpanded bool                      `json:"sessionsExpanded,omitempty"`
	Sessions         []SidebarLayoutSessionRef `json:"sessions,omitempty"`
}

type SidebarLayoutSessionRef struct {
	Name string `json:"name,omitempty"`
}

type SidebarLayoutSpacer struct {
	ID string `json:"id,omitempty"`
}

type SidebarState struct {
	ShowNumericSessions   bool            `json:"showNumericSessions,omitempty"`
	Open                  bool            `json:"open,omitempty"`
	OwnerClient           string          `json:"ownerClient,omitempty"`
	VisibleClients        map[string]bool `json:"visibleClients,omitempty"`
	AutoSortRecentRunAt   string          `json:"autoSortRecentRunAt,omitempty"`
	AutoSortRecentRunDate string          `json:"autoSortRecentRunDate,omitempty"`
}

type StateStoreUpdate func(state *PersistedState) error

type StateStorePort interface {
	Load(ctx context.Context, serverID string) (PersistedState, error)
	Save(ctx context.Context, serverID string, state PersistedState) error
}

type StateStoreUpdater interface {
	Update(ctx context.Context, serverID string, update StateStoreUpdate) error
}

func UpdateState(ctx context.Context, store StateStorePort, serverID string, update StateStoreUpdate) error {
	if updater, ok := store.(StateStoreUpdater); ok {
		return updater.Update(ctx, serverID, update)
	}
	state, err := store.Load(ctx, serverID)
	if err != nil {
		return err
	}
	previous := ClonePersistedState(state)
	if err := update(&state); err != nil {
		return err
	}
	if reflect.DeepEqual(previous, state) {
		return nil
	}
	return store.Save(ctx, serverID, state)
}

func ClonePersistedState(state PersistedState) PersistedState {
	clone := state
	clone.Sessions = maps.Clone(state.Sessions)
	clone.SessionOrder = cloneStringSlice(state.SessionOrder)
	clone.PinnedSessions = cloneStringSlice(state.PinnedSessions)
	clone.PinColors = maps.Clone(state.PinColors)
	clone.Sidebar = cloneSidebarState(state.Sidebar)
	clone.SidebarLayout = cloneSidebarLayout(state.SidebarLayout)
	clone.Clients = cloneByteMap(state.Clients)
	clone.Heat = cloneByteMap(state.Heat)
	clone.AgentAttention = cloneByteMap(state.AgentAttention)
	clone.Metadata = maps.Clone(state.Metadata)
	return clone
}

func cloneSidebarState(state *SidebarState) *SidebarState {
	if state == nil {
		return nil
	}
	clone := *state
	clone.VisibleClients = maps.Clone(state.VisibleClients)
	return &clone
}

func cloneSidebarLayout(layout *SidebarLayout) *SidebarLayout {
	if layout == nil {
		return nil
	}
	clone := &SidebarLayout{}
	if layout.Items == nil {
		return clone
	}
	clone.Items = make([]SidebarLayoutItem, len(layout.Items))
	for i, item := range layout.Items {
		clone.Items[i] = item
		if item.Category != nil {
			category := *item.Category
			category.Sessions = cloneSidebarLayoutSessionRefs(item.Category.Sessions)
			clone.Items[i].Category = &category
		}
		if item.Separator != nil {
			separator := *item.Separator
			clone.Items[i].Separator = &separator
		}
		if item.Spacer != nil {
			spacer := *item.Spacer
			clone.Items[i].Spacer = &spacer
		}
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func cloneSidebarLayoutSessionRefs(values []SidebarLayoutSessionRef) []SidebarLayoutSessionRef {
	if values == nil {
		return nil
	}
	return append([]SidebarLayoutSessionRef{}, values...)
}

func cloneByteMap(values map[string][]byte) map[string][]byte {
	if values == nil {
		return nil
	}
	clone := make(map[string][]byte, len(values))
	for key, value := range values {
		if value == nil {
			clone[key] = nil
			continue
		}
		clone[key] = append([]byte{}, value...)
	}
	return clone
}
