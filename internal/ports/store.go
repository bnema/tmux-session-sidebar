package ports

import "context"

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
	ShowNumericSessions   bool   `json:"showNumericSessions,omitempty"`
	Open                  bool   `json:"open,omitempty"`
	OwnerClient           string `json:"ownerClient,omitempty"`
	AutoSortRecentRunAt   string `json:"autoSortRecentRunAt,omitempty"`
	AutoSortRecentRunDate string `json:"autoSortRecentRunDate,omitempty"`
}

type StateStorePort interface {
	Load(ctx context.Context, serverID string) (PersistedState, error)
	Save(ctx context.Context, serverID string, state PersistedState) error
}
