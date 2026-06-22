package app

import (
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestCreateAdhocDuplicateNameSwitchDoesNotOverwriteMetadataOrCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	previous := ports.SessionMetadata{Kind: "captured", LastPath: "/tmp/existing/scratch"}
	seedPersistedState(t, map[string]ports.SessionMetadata{"scratch": previous}, []string{"scratch"})
	seedSidebarLayout(t, &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:existing", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:existing", Name: "Existing", Sessions: []ports.SidebarLayoutSessionRef{{Name: "scratch"}}}},
		{ID: "category:target", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:target", Name: "Target"}},
	}})
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\tscratch\t1\t1\n' ;;
  new-session) echo 'unexpected new-session' >&2; exit 42 ;;
esac
`)

	if err := createAdhoc(t.Context(), map[string]string{"source-path": "/tmp/new/scratch", "name": "scratch", "category-id": "category:target"}, nil); err != nil {
		t.Fatalf("createAdhoc returned error: %v\nlog=%q", err, readLog(t, logPath))
	}
	assertPersistedMetadata(t, "scratch", previous)
	assertCategorySessions(t, "category:existing", []string{"scratch"})
	assertCategorySessions(t, "category:target", []string{})
}

func TestCreateProjectExistingNameSwitchDoesNotOverwriteMetadataOrCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	previous := ports.SessionMetadata{Kind: "project", ProjectPath: "/tmp/existing/project", LastPath: "/tmp/existing/project"}
	seedPersistedState(t, map[string]ports.SessionMetadata{"project": previous}, []string{"project"})
	seedSidebarLayout(t, &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:existing", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:existing", Name: "Existing", Sessions: []ports.SidebarLayoutSessionRef{{Name: "project"}}}},
		{ID: "category:target", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:target", Name: "Target"}},
	}})
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\tproject\t1\t1\n' ;;
  new-session) echo 'unexpected new-session' >&2; exit 42 ;;
esac
`)

	if err := createProject(t.Context(), map[string]string{"project-path": "/tmp/new/project", "category-id": "category:target"}, nil, nil); err != nil {
		t.Fatalf("createProject returned error: %v\nlog=%q", err, readLog(t, logPath))
	}
	assertPersistedMetadata(t, "project", previous)
	assertCategorySessions(t, "category:existing", []string{"project"})
	assertCategorySessions(t, "category:target", []string{})
}

func assertCategorySessions(t *testing.T, categoryID string, want []string) {
	t.Helper()
	state, err := loadSidebarState(t.Context())
	if err != nil {
		t.Fatalf("loadSidebarState error = %v", err)
	}
	if state.SidebarLayout == nil {
		t.Fatalf("SidebarLayout = nil, want category %q", categoryID)
	}
	for _, item := range state.SidebarLayout.Items {
		if item.Category == nil || item.Category.ID != categoryID {
			continue
		}
		got := make([]string, 0, len(item.Category.Sessions))
		for _, ref := range item.Category.Sessions {
			got = append(got, ref.Name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("category %q sessions = %#v, want %#v", categoryID, got, want)
		}
		return
	}
	t.Fatalf("category %q not found in layout %#v", categoryID, state.SidebarLayout)
}

func seedSidebarLayout(t *testing.T, layout *ports.SidebarLayout) {
	t.Helper()
	if err := updateSidebarState(t.Context(), func(state *ports.PersistedState) {
		state.SidebarLayout = layout
	}); err != nil {
		t.Fatalf("seed sidebar layout error = %v", err)
	}
}
