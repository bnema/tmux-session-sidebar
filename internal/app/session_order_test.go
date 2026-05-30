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
		{name: "moves visible session above numeric first session", initial: nil, live: []string{"1", "alpha", "beta"}, session: "beta", delta: -1, want: []string{"1", "beta", "alpha"}},
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

func TestSaveToggledPinnedSession(t *testing.T) {
	ctx := context.Background()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.PinColors = map[string]string{"beta": "#38bdf8"}
	}); err != nil {
		t.Fatalf("seed PinColors error = %v", err)
	}
	if err := saveToggledPinnedSession(ctx, []string{"alpha", "beta"}, "beta"); err != nil {
		t.Fatalf("saveToggledPinnedSession() pin error = %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after pin error = %v", err)
	}
	if want := []string{"beta"}; !reflect.DeepEqual(state.PinnedSessions, want) {
		t.Fatalf("PinnedSessions after pin = %#v, want %#v", state.PinnedSessions, want)
	}
	if got := state.PinColors["beta"]; got != "#38bdf8" {
		t.Fatalf("PinColors[beta] after pin = %q, want preserved #38bdf8", got)
	}
	if err := saveToggledPinnedSession(ctx, []string{"alpha", "beta"}, "beta"); err != nil {
		t.Fatalf("saveToggledPinnedSession() unpin error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after unpin error = %v", err)
	}
	if len(state.PinnedSessions) != 0 {
		t.Fatalf("PinnedSessions after unpin = %#v, want empty", state.PinnedSessions)
	}
	if _, ok := state.PinColors["beta"]; ok {
		t.Fatalf("PinColors[beta] kept after unpin: %#v", state.PinColors)
	}

	if err := savePinnedSessionColor(ctx, []string{"alpha", "beta"}, "beta", "#38bdf8"); err != nil {
		t.Fatalf("savePinnedSessionColor() add error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after add error = %v", err)
	}
	if want := []string{"beta"}; !reflect.DeepEqual(state.PinnedSessions, want) {
		t.Fatalf("PinnedSessions after add = %#v, want %#v", state.PinnedSessions, want)
	}
	if got := state.PinColors["beta"]; got != "#38bdf8" {
		t.Fatalf("PinColors[beta] after add = %q, want #38bdf8", got)
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder after add = %#v, want %#v", state.SessionOrder, want)
	}

	if err := savePinnedSessionColor(ctx, []string{"alpha", "beta"}, "beta", "#f87171"); err != nil {
		t.Fatalf("savePinnedSessionColor() recolor error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after recolor error = %v", err)
	}
	if want := []string{"beta"}; !reflect.DeepEqual(state.PinnedSessions, want) {
		t.Fatalf("PinnedSessions after recolor = %#v, want %#v", state.PinnedSessions, want)
	}
	if got := state.PinColors["beta"]; got != "#f87171" {
		t.Fatalf("PinColors[beta] after recolor = %q, want #f87171", got)
	}

	if err := saveToggledPinnedSession(ctx, []string{"alpha", "beta"}, "beta"); err != nil {
		t.Fatalf("saveToggledPinnedSession() remove error = %v", err)
	}
	state, err = loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after remove error = %v", err)
	}
	if len(state.PinnedSessions) != 0 {
		t.Fatalf("PinnedSessions after remove = %#v, want empty", state.PinnedSessions)
	}
	if _, ok := state.PinColors["beta"]; ok {
		t.Fatalf("PinColors[beta] kept after remove: %#v", state.PinColors)
	}
}

func TestSavePinnedSessionColorIgnoresNonLiveSession(t *testing.T) {
	ctx := context.Background()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.PinnedSessions = []string{"missing"}
		state.PinColors = map[string]string{"alpha": "#38bdf8"}
	}); err != nil {
		t.Fatalf("seed sidebar state error = %v", err)
	}
	if err := savePinnedSessionColor(ctx, []string{"alpha", "beta"}, "missing", "#f87171"); err != nil {
		t.Fatalf("savePinnedSessionColor() non-live error = %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() after non-live pin color error = %v", err)
	}
	if len(state.PinnedSessions) != 0 {
		t.Fatalf("PinnedSessions after non-live color = %#v, want empty after reconciliation", state.PinnedSessions)
	}
	if _, ok := state.PinColors["missing"]; ok {
		t.Fatalf("PinColors[missing] was added for non-live session: %#v", state.PinColors)
	}
	if got := state.PinColors["alpha"]; got != "#38bdf8" {
		t.Fatalf("PinColors[alpha] = %q, want untouched #38bdf8", got)
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
	state.PinnedSessions = []string{"alpha"}
	state.PinColors = map[string]string{"alpha": "#38bdf8"}
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
	if want := []string{"beta"}; !reflect.DeepEqual(state.PinnedSessions, want) {
		t.Fatalf("PinnedSessions after rename = %#v, want %#v", state.PinnedSessions, want)
	}
	if got := state.PinColors["beta"]; got != "#38bdf8" {
		t.Fatalf("PinColors[beta] after rename = %q, want #38bdf8", got)
	}
	if _, ok := state.PinColors["alpha"]; ok {
		t.Fatalf("PinColors[alpha] still exists after rename: %#v", state.PinColors)
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
	if len(state.PinnedSessions) != 0 {
		t.Fatalf("PinnedSessions after numeric rename = %#v, want empty", state.PinnedSessions)
	}
	if _, ok := state.PinColors["beta"]; ok {
		t.Fatalf("PinColors[beta] kept after numeric rename: %#v", state.PinColors)
	}

	// Renaming a non-existent session to a hidden name is a no-op and must not create hidden restore metadata.
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

func TestClonePersistedStatePreservesPinnedSessionsAndAgentAttention(t *testing.T) {
	original := ports.PersistedState{
		PinnedSessions: []string{"alpha"},
		PinColors:      map[string]string{"alpha": "#38bdf8"},
		AgentAttention: map[string][]byte{"$1": []byte("attention")},
	}

	clone := clonePersistedState(original)
	if !reflect.DeepEqual(clone.PinnedSessions, original.PinnedSessions) {
		t.Fatalf("PinnedSessions = %#v, want %#v", clone.PinnedSessions, original.PinnedSessions)
	}
	clone.PinnedSessions[0] = "beta"
	if original.PinnedSessions[0] != "alpha" {
		t.Fatalf("original PinnedSessions mutated: %#v", original.PinnedSessions)
	}
	if !reflect.DeepEqual(clone.PinColors, original.PinColors) {
		t.Fatalf("PinColors = %#v, want %#v", clone.PinColors, original.PinColors)
	}
	clone.PinColors["alpha"] = "#f87171"
	if original.PinColors["alpha"] != "#38bdf8" {
		t.Fatalf("original PinColors mutated: %#v", original.PinColors)
	}
	if !reflect.DeepEqual(clone.AgentAttention, original.AgentAttention) {
		t.Fatalf("AgentAttention = %#v, want %#v", clone.AgentAttention, original.AgentAttention)
	}
	clone.AgentAttention["$1"][0] = 'A'
	if string(original.AgentAttention["$1"]) != "attention" {
		t.Fatalf("original AgentAttention mutated: %q", string(original.AgentAttention["$1"]))
	}
}

func TestSaveShowNumericSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState() error = %v", err)
	}
	if err := saveShowNumericSessions(ctx, true); err != nil {
		t.Fatalf("saveShowNumericSessions() error = %v", err)
	}
	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState() error = %v", err)
	}
	if state.Sidebar == nil || !state.Sidebar.ShowNumericSessions {
		t.Fatalf("ShowNumericSessions = false, want true")
	}
	if !state.Sidebar.Open || state.Sidebar.OwnerClient != "client-1" {
		t.Fatalf("sidebar state = %#v, want open owner preserved", state.Sidebar)
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
