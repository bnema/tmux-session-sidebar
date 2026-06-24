package ports

import (
	"context"
	"reflect"
	"testing"
)

func TestUpdateStateDetectsInPlaceNestedMutations(t *testing.T) {
	store := &recordingStore{state: PersistedState{
		Sessions: map[string]SessionMetadata{"alpha": {Kind: "project"}},
		Clients:  map[string][]byte{"%1": []byte(`{"currentSessionId":"$1"}`)},
		Sidebar:  &SidebarState{Open: true},
		SidebarLayout: &SidebarLayout{Items: []SidebarLayoutItem{{
			Category: &SidebarLayoutCategory{Sessions: []SidebarLayoutSessionRef{{Name: "alpha"}}},
		}}},
	}}

	err := UpdateState(context.Background(), store, "server", func(state *PersistedState) error {
		state.Sessions["alpha"] = SessionMetadata{Kind: "captured", LastPath: "/tmp/alpha"}
		state.Clients["%1"][0] = '['
		state.Sidebar.Open = false
		state.SidebarLayout.Items[0].Category.Sessions[0].Name = "beta"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateState error: %v", err)
	}
	if store.saves != 1 {
		t.Fatalf("Save calls = %d, want 1", store.saves)
	}
	if got := store.state.Sessions["alpha"].LastPath; got != "/tmp/alpha" {
		t.Fatalf("saved LastPath = %q, want /tmp/alpha", got)
	}
}

func TestUpdateStateSkipsSaveWhenStateUnchanged(t *testing.T) {
	initial := PersistedState{Sessions: map[string]SessionMetadata{"alpha": {Kind: "project"}}}
	store := &recordingStore{state: initial}

	if err := UpdateState(context.Background(), store, "server", func(state *PersistedState) error { return nil }); err != nil {
		t.Fatalf("UpdateState error: %v", err)
	}
	if store.saves != 0 {
		t.Fatalf("Save calls = %d, want 0", store.saves)
	}
	if !reflect.DeepEqual(store.state, initial) {
		t.Fatalf("state = %#v, want %#v", store.state, initial)
	}
}

type recordingStore struct {
	state PersistedState
	saves int
}

func (s *recordingStore) Load(context.Context, string) (PersistedState, error) {
	return s.state, nil
}

func (s *recordingStore) Save(_ context.Context, _ string, state PersistedState) error {
	s.saves++
	s.state = state
	return nil
}
