package ports

import "context"

type PersistedState struct {
	Sessions     map[string]SessionMetadata
	SessionOrder []string
	Clients      map[string][]byte
	Heat         map[string][]byte
}

type StateStorePort interface {
	Load(ctx context.Context, serverID string) (PersistedState, error)
	Save(ctx context.Context, serverID string, state PersistedState) error
}
