package app

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
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
