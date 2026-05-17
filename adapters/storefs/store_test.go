package storefs

import (
	"context"
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
			if len(got.Sessions) != len(tt.state.Sessions) || len(got.Clients) != len(tt.state.Clients) || len(got.Heat) != len(tt.state.Heat) {
				t.Fatalf("loaded state = %#v, want %#v", got, tt.state)
			}
		})
	}
}
