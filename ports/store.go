package ports

import "context"

type PersistedState struct {
	Sessions       map[string]SessionMetadata `json:"sessions,omitempty"`
	SessionOrder   []string                   `json:"sessionOrder,omitempty"`
	PinnedSessions []string                   `json:"pinnedSessions,omitempty"`
	PinColors      map[string]string          `json:"pinColors,omitempty"`
	Sidebar        *SidebarState              `json:"sidebar,omitempty"`
	Clients        map[string][]byte          `json:"clients,omitempty"`
	Heat           map[string][]byte          `json:"heat,omitempty"`
	AgentAttention map[string][]byte          `json:"agentAttention,omitempty"`
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
