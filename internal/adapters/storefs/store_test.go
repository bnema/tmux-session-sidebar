package storefs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/core/persisted"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestStoreLoadSave(t *testing.T) {
	tests := []struct {
		name  string
		state ports.PersistedState
	}{
		{name: "empty maps", state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}, SessionOrder: []string{}, Clients: map[string][]byte{}, Heat: map[string][]byte{}}},
		{name: "session metadata", state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{"alpha": {Kind: "project", ProjectPath: "/tmp/alpha"}}, SessionOrder: []string{}, PinnedSessions: []string{"alpha"}, Clients: map[string][]byte{"%1": []byte("{}")}, Heat: map[string][]byte{"alpha": []byte("{}")}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := New(t.TempDir())
			ctx := context.Background()
			if err := store.Save(ctx, "server", tt.state); err != nil {
				t.Fatalf("Save error: %v", err)
			}
			got, err := store.Load(ctx, "server")
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			want := tt.state
			persisted.InitializeState(&want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("loaded state = %#v, want %#v", got, want)
			}
		})
	}
}

func TestStoreLoadSaveSessionRestoreMetadata(t *testing.T) {
	store := New(t.TempDir())
	ctx := context.Background()
	state := ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{
			"alpha": {Kind: "project", ProjectPath: "/tmp/alpha", LastPath: "/tmp/alpha/subdir"},
			"beta":  {Kind: "captured", LastPath: "/tmp/beta"},
		},
		SessionOrder:   []string{"alpha", "beta"},
		PinnedSessions: []string{"beta"},
		Clients:        map[string][]byte{},
		Heat:           map[string][]byte{},
	}
	if err := store.Save(ctx, "server", state); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, err := store.Load(ctx, "server")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	want := state
	persisted.InitializeState(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded state = %#v, want %#v", got, want)
	}
}

func TestStoreSavePreservesUnknownTopLevelFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")
	original := `{
  "sessions": {
    "alpha": {"kind": "project", "projectPath": "/tmp/alpha"}
  },
  "sidebar": {"showNumericSessions": true, "nestedFuture": {"discard": true}},
  "sidebarLayout": {
    "items": [
      {
        "id": "category:work",
        "kind": "category",
        "category": {
          "id": "category:work",
          "name": "Work",
          "sessions": [{"name": "alpha"}]
        }
      }
    ]
  },
  "futureState": {"nested": true},
  "futureArray": [1, {"two": 2}]
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	store := New(dir)
	state, err := store.Load(context.Background(), "server")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	state.SessionOrder = []string{"alpha"}
	state.Sidebar = &ports.SidebarState{Open: true}
	if err := store.Save(context.Background(), "server", state); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("Unmarshal saved state error: %v", err)
	}
	assertRawJSONEqual(t, saved["futureState"], `{"nested": true}`)
	assertRawJSONEqual(t, saved["futureArray"], `[1, {"two": 2}]`)
	assertRawJSONEqual(t, saved["sessionOrder"], `["alpha"]`)
	assertRawJSONEqual(t, saved["sidebar"], `{"open": true}`)
}

func assertRawJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("missing JSON field, want %s", want)
	}
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("Unmarshal got JSON error: %v", err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("Unmarshal want JSON error: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestStoreSaveWritesTinyAtomicJSON(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	state := ports.PersistedState{SessionOrder: []string{"beta", "alpha"}, Sidebar: &ports.SidebarState{ShowNumericSessions: true}}
	if err := store.Save(context.Background(), "server", state); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "server.json" {
		t.Fatalf("store dir entries = %#v, want only server.json", entries)
	}
	data, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	content := string(data)
	for _, absent := range []string{"\"sessions\"", "\"clients\"", "\"heat\""} {
		if strings.Contains(content, absent) {
			t.Fatalf("tiny json should omit %q, got %s", absent, content)
		}
	}
	for _, want := range []string{"sessionOrder", "sidebar", "showNumericSessions"} {
		if !strings.Contains(content, want) {
			t.Fatalf("json missing %q: %s", want, content)
		}
	}
}

func TestStoreLoadEdges(t *testing.T) {
	tests := []struct {
		name      string
		serverID  string
		setup     func(string) error
		wantErr   bool
		wantEmpty bool
	}{
		{name: "missing server initializes maps", serverID: "missing", wantEmpty: true},
		{name: "sparse json", serverID: "sparse", setup: func(dir string) error { return os.WriteFile(filepath.Join(dir, "sparse.json"), []byte("{}"), 0o600) }, wantEmpty: true},
		{name: "invalid json", serverID: "bad", setup: func(dir string) error { return os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600) }, wantErr: true},
		{name: "path traversal confined", serverID: "../evil", wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				if err := tt.setup(dir); err != nil {
					t.Fatalf("setup error: %v", err)
				}
			}
			got, err := New(dir).Load(context.Background(), tt.serverID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantEmpty && (got.Sessions == nil || got.SessionOrder == nil || got.Clients == nil || got.Heat == nil) {
				t.Fatalf("expected initialized maps, got %#v", got)
			}
		})
	}
}
