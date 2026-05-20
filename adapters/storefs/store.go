package storefs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Store struct {
	Dir string
}

func New(dir string) Store { return Store{Dir: dir} }

func (s Store) Load(_ context.Context, serverID string) (ports.PersistedState, error) {
	path := s.path(serverID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}, SessionOrder: []string{}, Clients: map[string][]byte{}, Heat: map[string][]byte{}}, nil
	}
	if err != nil {
		return ports.PersistedState{}, err
	}
	var state ports.PersistedState
	if err := decodePersistedState(data, &state); err != nil {
		return ports.PersistedState{}, err
	}
	initializeMaps(&state)
	return state, nil
}

func (s Store) Save(_ context.Context, serverID string, state ports.PersistedState) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(serverID)
	tmp, err := os.CreateTemp(s.Dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(s.Dir)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func (s Store) path(serverID string) string {
	return filepath.Join(s.Dir, filepath.Base(serverID)+".json")
}

type legacyPersistedState struct {
	Sessions     map[string]ports.SessionMetadata `json:"Sessions"`
	SessionOrder []string                         `json:"SessionOrder"`
	Sidebar      *legacySidebarState              `json:"Sidebar"`
	Clients      map[string][]byte                `json:"Clients"`
	Heat         map[string][]byte                `json:"Heat"`
}

type legacySidebarState struct {
	ShowNumericSessions bool `json:"ShowNumericSessions"`
}

// decodePersistedState first unmarshals current camelCase keys, then overlays
// legacyPersistedState to migrate legacy PascalCase top-level keys such as
// Sessions, SessionOrder, Sidebar, Clients, and Heat.
func decodePersistedState(data []byte, state *ports.PersistedState) error {
	if err := json.Unmarshal(data, state); err != nil {
		return err
	}
	var legacy legacyPersistedState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	if len(state.Sessions) == 0 && legacy.Sessions != nil {
		state.Sessions = legacy.Sessions
	}
	if len(state.SessionOrder) == 0 && legacy.SessionOrder != nil {
		state.SessionOrder = legacy.SessionOrder
	}
	if state.Sidebar == nil && legacy.Sidebar != nil {
		state.Sidebar = &ports.SidebarState{ShowNumericSessions: legacy.Sidebar.ShowNumericSessions}
	}
	if len(state.Clients) == 0 && legacy.Clients != nil {
		state.Clients = legacy.Clients
	}
	if len(state.Heat) == 0 && legacy.Heat != nil {
		state.Heat = legacy.Heat
	}
	return nil
}

func initializeMaps(state *ports.PersistedState) {
	if state.Sessions == nil {
		state.Sessions = map[string]ports.SessionMetadata{}
	}
	if state.SessionOrder == nil {
		state.SessionOrder = []string{}
	}
	if state.Clients == nil {
		state.Clients = map[string][]byte{}
	}
	if state.Heat == nil {
		state.Heat = map[string][]byte{}
	}
}
