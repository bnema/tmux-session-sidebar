package app

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSaveMovedSessionOrder(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		live    []string
		session string
		delta   int
		want    []string
	}{
		{name: "moves selected down after applying saved order", initial: []string{"gamma", "alpha", "beta"}, live: []string{"alpha", "beta", "gamma"}, session: "alpha", delta: 1, want: []string{"gamma", "beta", "alpha"}},
		{name: "clamps first session moving up", initial: nil, live: []string{"alpha", "beta", "gamma"}, session: "alpha", delta: -1, want: []string{"alpha", "beta", "gamma"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			store := sessionOrderStore()
			state, err := store.Load(context.Background(), "tmux")
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			state.SessionOrder = tt.initial
			if err := store.Save(context.Background(), "tmux", state); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			if err := saveMovedSessionOrder(context.Background(), tt.live, tt.session, tt.delta); err != nil {
				t.Fatalf("saveMovedSessionOrder() error = %v", err)
			}
			got, err := store.Load(context.Background(), "tmux")
			if err != nil {
				t.Fatalf("Load() after move error = %v", err)
			}
			if !reflect.DeepEqual(got.SessionOrder, tt.want) {
				t.Fatalf("SessionOrder = %#v, want %#v", got.SessionOrder, tt.want)
			}
		})
	}
}

func TestSessionMetadataPersistenceHelpers(t *testing.T) {
	ctx := context.Background()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sessions = map[string]ports.SessionMetadata{
		"alpha": {Kind: "project", ProjectPath: "/tmp/alpha", LastPath: "/tmp/alpha"},
	}
	state.SessionOrder = []string{"gamma", "alpha"}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	metadata := ports.SessionMetadata{Kind: "adhoc", LastPath: "/tmp/scratch"}
	if err := saveSessionMetadata(ctx, "alpha", metadata); err != nil {
		t.Fatalf("saveSessionMetadata() error = %v", err)
	}
	gotMetadata, ok := persistedSessionMetadata(ctx, "alpha")
	if !ok || gotMetadata != metadata {
		t.Fatalf("persistedSessionMetadata() = %#v, %v; want %#v, true", gotMetadata, ok, metadata)
	}

	if err := renamePersistedSession(ctx, "alpha", "beta"); err != nil {
		t.Fatalf("renamePersistedSession() error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after rename error = %v", err)
	}
	if _, ok := state.Sessions["alpha"]; ok {
		t.Fatalf("Sessions[alpha] still exists after rename: %#v", state.Sessions)
	}
	if state.Sessions["beta"] != metadata {
		t.Fatalf("Sessions[beta] = %#v, want %#v", state.Sessions["beta"], metadata)
	}
	if want := []string{"gamma", "beta"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder after rename = %#v, want %#v", state.SessionOrder, want)
	}

	if err := renamePersistedSession(ctx, "beta", "123"); err != nil {
		t.Fatalf("renamePersistedSession() to numeric error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after numeric rename error = %v", err)
	}
	if _, ok := state.Sessions["beta"]; ok {
		t.Fatalf("Sessions[beta] still exists after numeric rename: %#v", state.Sessions)
	}
	if _, ok := state.Sessions["123"]; ok {
		t.Fatalf("Sessions[123] exists after numeric rename: %#v", state.Sessions)
	}
	if want := []string{"gamma"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder after numeric rename = %#v, want %#v", state.SessionOrder, want)
	}

	if err := renamePersistedSession(ctx, "alpha", "__hidden"); err != nil {
		t.Fatalf("renamePersistedSession() to hidden error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after hidden rename error = %v", err)
	}
	if _, ok := state.Sessions["alpha"]; ok {
		t.Fatalf("Sessions[alpha] exists after hidden rename: %#v", state.Sessions)
	}
	if _, ok := state.Sessions["__hidden"]; ok {
		t.Fatalf("Sessions[__hidden] exists after hidden rename: %#v", state.Sessions)
	}
	if want := []string{"gamma"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder after hidden rename = %#v, want %#v", state.SessionOrder, want)
	}

	if err := saveSessionMetadata(ctx, "beta", metadata); err != nil {
		t.Fatalf("saveSessionMetadata() beta error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after beta recreate error = %v", err)
	}
	state.SessionOrder = []string{"gamma", "beta"}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("Save() beta order error = %v", err)
	}

	if err := removePersistedSession(ctx, "beta"); err != nil {
		t.Fatalf("removePersistedSession() error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after remove error = %v", err)
	}
	if _, ok := state.Sessions["beta"]; ok {
		t.Fatalf("Sessions[beta] still exists after remove: %#v", state.Sessions)
	}
	if want := []string{"gamma"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder after remove = %#v, want %#v", state.SessionOrder, want)
	}
}

func TestSaveShowNumericSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := saveShowNumericSessions(context.Background(), true); err != nil {
		t.Fatalf("saveShowNumericSessions() error = %v", err)
	}
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState() error = %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.ShowNumericSessions {
		t.Fatalf("ShowNumericSessions = false, want true")
	}
}

func TestSessionOrderStoreResolvesStateDirectory(t *testing.T) {
	tests := []struct {
		name          string
		xdgStateHome  string
		wantDirSuffix string
	}{
		{name: "uses XDG_STATE_HOME when set", xdgStateHome: filepath.Join("tmp", "state"), wantDirSuffix: filepath.Join("tmp", "state", "tmux-session-sidebar")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), tt.xdgStateHome)
			t.Setenv("XDG_STATE_HOME", base)
			got := sessionOrderStore()
			want := filepath.Join(base, "tmux-session-sidebar")
			if got.Dir != want {
				t.Fatalf("Dir = %q, want %q", got.Dir, want)
			}
		})
	}
}
