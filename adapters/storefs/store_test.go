package storefs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
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
			if !reflect.DeepEqual(got, tt.state) {
				t.Fatalf("loaded state = %#v, want %#v", got, tt.state)
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
	if !reflect.DeepEqual(got, state) {
		t.Fatalf("loaded state = %#v, want %#v", got, state)
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
