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
	if err := json.Unmarshal(data, &state); err != nil {
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
