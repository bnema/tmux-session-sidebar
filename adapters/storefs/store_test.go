package storefs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestStoreLoadSave(t *testing.T) {
	tests := []struct {
		name  string
		state ports.PersistedState
	}{
		{name: "empty maps", state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{}, Clients: map[string][]byte{}, Heat: map[string][]byte{}}},
		{name: "session metadata", state: ports.PersistedState{Sessions: map[string]ports.SessionMetadata{"alpha": {Kind: "project", ProjectPath: "/tmp/alpha"}}, Clients: map[string][]byte{"%1": []byte("{}")}, Heat: map[string][]byte{"alpha": []byte("{}")}}},
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
			if tt.wantEmpty && (got.Sessions == nil || got.Clients == nil || got.Heat == nil) {
				t.Fatalf("expected initialized maps, got %#v", got)
			}
		})
	}
}
