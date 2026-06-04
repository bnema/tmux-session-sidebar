package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSidebarLayoutPersistenceRoundTrip(t *testing.T) {
	core := sidebarlayout.Layout{Items: []sidebarlayout.LayoutItem{
		sidebarlayout.CategoryItem("category:work", "Work", false, []string{"alpha", "beta"}),
		sidebarlayout.SeparatorItem("separator:one"),
		sidebarlayout.SpacerItem("spacer:one"),
	}}

	got := coreLayoutFromPersisted(persistedLayoutFromCore(core))

	if !reflect.DeepEqual(got, core) {
		t.Fatalf("round trip = %#v, want %#v", got, core)
	}
}

func TestSaveNewSidebarLayoutItemsAndMove(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha", "beta"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveNewSidebarCategory(ctx, "Work", live); err != nil {
		t.Fatalf("saveNewSidebarCategory error: %v", err)
	}
	if err := saveNewSidebarSeparator(ctx, live); err != nil {
		t.Fatalf("saveNewSidebarSeparator error: %v", err)
	}
	if err := saveNewSidebarSpacer(ctx, live); err != nil {
		t.Fatalf("saveNewSidebarSpacer error: %v", err)
	}
	if err := saveMovedSidebarLayoutItem(ctx, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSeparator, ItemID: "separator:1"}, -1, live); err != nil {
		t.Fatalf("saveMovedSidebarLayoutItem error: %v", err)
	}

	state, err := loadSidebarState(ctx)
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if got := layoutKinds(layout); !reflect.DeepEqual(got, []sidebarlayout.ItemKind{sidebarlayout.ItemKindCategory, sidebarlayout.ItemKindSeparator, sidebarlayout.ItemKindCategory, sidebarlayout.ItemKindSpacer}) {
		t.Fatalf("layout kinds = %#v, want default, separator, work, spacer", got)
	}
}

func TestLoadSidebarTreeItemsMigratesDefaultAndContextualMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, sidebarTreeFakeTmuxScript("off"))
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SessionOrder = []string{"beta", "alpha"}
	state.Metadata = map[string]ports.GitStatus{"alpha": {Branch: "main", Clean: true}, "beta": {Branch: "dev", Clean: true}}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSidebarTreeItemsWithConfig(ctx, loadSidebarConfig(ctx))
	if err != nil {
		t.Fatalf("loadSidebarTreeItemsWithConfig error: %v", err)
	}

	var alpha, beta sidebarTreeItem
	for _, item := range items {
		if item.Kind == sidebarTreeRowSession && item.Session.Name == "alpha" {
			alpha = item
		}
		if item.Kind == sidebarTreeRowSession && item.Session.Name == "beta" {
			beta = item
		}
	}
	if alpha.Session.Metadata.Branch != "main" || !alpha.ShowMetadata || alpha.Slot != 1 {
		t.Fatalf("alpha tree item = %#v, want active category metadata and slot 1", alpha)
	}
	if beta.Session.Metadata.Branch != "dev" || beta.ShowMetadata || beta.Slot != 0 {
		t.Fatalf("beta tree item = %#v, want inactive metadata hidden and no contextual slot", beta)
	}
}

func TestLoadSidebarTreeItemsCanShowInactiveMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, sidebarTreeFakeTmuxScript("on"))
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Metadata = map[string]ports.GitStatus{"beta": {Branch: "dev", Clean: true}}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
		{ID: "category:default", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:default", Name: "Default", Sessions: []ports.SidebarLayoutSessionRef{{Name: "beta"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSidebarTreeItemsWithConfig(ctx, loadSidebarConfig(ctx))
	if err != nil {
		t.Fatalf("loadSidebarTreeItemsWithConfig error: %v", err)
	}

	for _, item := range items {
		if item.Kind == sidebarTreeRowSession && item.Session.Name == "beta" && !item.ShowMetadata {
			t.Fatalf("beta ShowMetadata = false, want true when inactive metadata enabled: %#v", item)
		}
	}
}

func TestContextualQuickSwitchTargetUsesCurrentSessionCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  display-message) printf 'beta\n' ;;
esac
`)
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
		{ID: "category:default", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:default", Name: "Default", Sessions: []ports.SidebarLayoutSessionRef{{Name: "beta"}, {Name: "gamma"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	target, err := contextualQuickSwitchTarget(ctx, []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}, {Name: "gamma", Visible: true}}, 2)
	if err != nil {
		t.Fatalf("contextualQuickSwitchTarget error: %v", err)
	}
	if target != "gamma" {
		t.Fatalf("target = %q, want gamma from current category", target)
	}
}

func TestSidebarLayoutActionDoesNotClearLayoutWhenLiveSessionsFail(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  list-sessions) printf 'tmux unavailable\n' >&2; exit 1 ;;
  display-message) ;;
esac
`)
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	actions := buildSidebarActions(ctx, map[string]string{}, nil, nil, nil)
	if ok := actions.CreateSpacer(); ok {
		t.Fatal("CreateSpacer = true, want false when live sessions fail")
	}

	got, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if !reflect.DeepEqual(got.SidebarLayout, state.SidebarLayout) {
		t.Fatalf("SidebarLayout changed after live-session failure: got %#v want %#v", got.SidebarLayout, state.SidebarLayout)
	}
}

func TestSidebarCategoryRenameRejectsMissingCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := saveReconciledSidebarLayout(ctx, []string{"alpha"}); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}

	if err := saveRenamedSidebarCategory(ctx, "category:missing", "Missing", []string{"alpha"}); err == nil {
		t.Fatal("saveRenamedSidebarCategory missing category error = nil, want error")
	}
}

func TestSidebarCategoryNameValidationRejectsEmptyNames(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := saveNewSidebarCategory(ctx, "  ", []string{"alpha"}); err == nil {
		t.Fatal("saveNewSidebarCategory empty name error = nil, want error")
	}
	if err := saveRenamedSidebarCategory(ctx, "category:default", "\t", []string{"alpha"}); err == nil {
		t.Fatal("saveRenamedSidebarCategory empty name error = nil, want error")
	}
}

func layoutKinds(layout sidebarlayout.Layout) []sidebarlayout.ItemKind {
	kinds := make([]sidebarlayout.ItemKind, 0, len(layout.Items))
	for _, item := range layout.Items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}

func sidebarTreeFakeTmuxScript(metadataInactive string) string {
	return `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'off\n' ;;
      @session-sidebar-agent-attention-animation) printf 'pulse\n' ;;
      @session-sidebar-auto-sort-recent) printf 'off\n' ;;
      @session-sidebar-restore-sessions) printf 'auto\n' ;;
      @session-sidebar-continuum-grace-seconds) printf '3\n' ;;
      @session-sidebar-metadata-subline) printf 'on\n' ;;
      @session-sidebar-metadata-inactive) printf '` + metadataInactive + `\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\n' ;;
  list-sessions) printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n' ;;
  list-clients) ;;
esac
`
}
