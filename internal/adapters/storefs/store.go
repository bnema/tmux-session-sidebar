package storefs

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/persisted"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type Store struct {
	Dir string
}

func New(dir string) Store { return Store{Dir: dir} }

func (s Store) Load(_ context.Context, serverID string) (ports.PersistedState, error) {
	path := s.path(serverID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return persisted.EmptyState(), nil
	}
	if err != nil {
		return ports.PersistedState{}, err
	}
	return persisted.DecodeState(data)
}

func (s Store) Save(_ context.Context, serverID string, state ports.PersistedState) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	path := s.path(serverID)
	data, err := marshalStatePreservingUnknownFields(path, state)
	if err != nil {
		return err
	}
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

func (s Store) Update(ctx context.Context, serverID string, update ports.StateStoreUpdate) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	lock, err := s.acquireUpdateLock(ctx, serverID)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	state, err := s.Load(ctx, serverID)
	if err != nil {
		return err
	}
	previous := ports.ClonePersistedState(state)
	if err := update(&state); err != nil {
		return err
	}
	if reflect.DeepEqual(previous, state) {
		return nil
	}
	return s.Save(ctx, serverID, state)
}

func (s Store) acquireUpdateLock(ctx context.Context, serverID string) (*os.File, error) {
	path := filepath.Join(s.Dir, filepath.Base(serverID)+".update.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		default:
			time.Sleep(25 * time.Millisecond)
		}
	}
}

func (s Store) path(serverID string) string {
	return filepath.Join(s.Dir, filepath.Base(serverID)+".json")
}

func marshalStatePreservingUnknownFields(path string, state ports.PersistedState) ([]byte, error) {
	current, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	merged := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		var existing map[string]json.RawMessage
		if err := json.Unmarshal(data, &existing); err == nil {
			for key, value := range existing {
				if !knownPersistedStateKey(key) {
					merged[key] = value
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(current, &known); err != nil {
		return nil, err
	}
	maps.Copy(merged, known)
	return json.MarshalIndent(merged, "", "  ")
}

var knownPersistedStateKeys = persistedStateJSONKeys()

func knownPersistedStateKey(key string) bool {
	_, ok := knownPersistedStateKeys[key]
	return ok
}

func persistedStateJSONKeys() map[string]struct{} {
	typeOfState := reflect.TypeFor[ports.PersistedState]()
	keys := make(map[string]struct{}, typeOfState.NumField())
	for field := range typeOfState.Fields() {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		switch name {
		case "-":
			continue
		case "":
			name = field.Name
		}
		keys[name] = struct{}{}
	}
	return keys
}
