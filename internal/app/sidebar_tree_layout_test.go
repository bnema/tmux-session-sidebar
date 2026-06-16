package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/internal/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestSidebarLayoutPersistenceRoundTrip(t *testing.T) {
	core := sidebarlayout.Layout{Items: []sidebarlayout.LayoutItem{
		sidebarlayout.CategoryItemWithSessionExpansion("category:work", "Work", false, true, []string{"alpha", "beta"}),
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
	if len(layout.Items) != 4 {
		t.Fatalf("layout item count = %d, want 4: %#v", len(layout.Items), layout.Items)
	}
	if got := layout.Items[0]; got.ID != "category:default" || got.Category.Name != "Default" || !reflect.DeepEqual(sessionNamesFromRefs(got.Category.Sessions), []string{"alpha", "beta"}) {
		t.Fatalf("default category = %#v, want alpha,beta", got)
	}
	if got := layout.Items[1]; got.ID != "separator:1" || got.Kind != sidebarlayout.ItemKindSeparator {
		t.Fatalf("moved separator = %#v, want separator:1 at index 1", got)
	}
	if got := layout.Items[2]; got.ID != "category:1" || got.Category.Name != "Work" || len(got.Category.Sessions) != 0 {
		t.Fatalf("work category = %#v, want empty category:1 Work", got)
	}
	if got := layout.Items[3]; got.ID != "spacer:1" || got.Kind != sidebarlayout.ItemKindSpacer {
		t.Fatalf("spacer = %#v, want spacer:1 at index 3", got)
	}
}

func sessionNamesFromRefs(refs []sidebarlayout.SessionRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return names
}

func findSidebarTreeSession(items []uity.TreeItem, name string) (uity.TreeItem, bool) {
	for _, item := range items {
		if item.Kind == uity.TreeRowSession && item.Session.Name == name {
			return item, true
		}
	}
	return uity.TreeItem{}, false
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

	var alpha, beta uity.TreeItem
	for _, item := range items {
		if item.Kind == uity.TreeRowSession && item.Session.Name == "alpha" {
			alpha = item
		}
		if item.Kind == uity.TreeRowSession && item.Session.Name == "beta" {
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

func TestLoadSidebarTreeItemsShowsLoadingMetadataWhenCurrentSessionIsSidebar(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, strings.Replace(sidebarTreeFakeTmuxScript("off"), "display-message) printf 'alpha\\n' ;;", "display-message) printf 'tmux-session-sidebar\\n' ;;", 1))
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:default", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:default", Name: "Default", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSidebarTreeItemsWithConfig(ctx, loadSidebarConfig(ctx))
	if err != nil {
		t.Fatalf("loadSidebarTreeItemsWithConfig error: %v", err)
	}
	alpha, found := findSidebarTreeSession(items, "alpha")
	if !found {
		t.Fatalf("alpha row not found in %#v", items)
	}
	if !alpha.ShowMetadata || alpha.Session.Metadata.Kind != uity.MetadataKindLoading || alpha.Slot != 1 {
		t.Fatalf("alpha metadata = %#v show=%v slot=%d, want active fallback loading metadata", alpha.Session.Metadata, alpha.ShowMetadata, alpha.Slot)
	}
}

func TestLoadSidebarTreeItemsShowsLoadingMetadataLineWhileGitIsPending(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, sidebarTreeFakeTmuxScript("off"))
	ctx := context.Background()
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:default", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:default", Name: "Default", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSidebarTreeItemsWithConfig(ctx, loadSidebarConfig(ctx))
	if err != nil {
		t.Fatalf("loadSidebarTreeItemsWithConfig error: %v", err)
	}
	alpha, found := findSidebarTreeSession(items, "alpha")
	if !found {
		t.Fatalf("alpha row not found in %#v", items)
	}
	if !alpha.ShowMetadata || alpha.Session.Metadata.Kind != uity.MetadataKindLoading {
		t.Fatalf("alpha metadata = %#v show=%v, want immediate loading metadata line", alpha.Session.Metadata, alpha.ShowMetadata)
	}
}

func TestLoadSidebarTreeItemsShowsInactiveMetadataByDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, sidebarTreeFakeTmuxScript(""))
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
	beta, found := findSidebarTreeSession(items, "beta")
	if !found || beta.Session.Metadata.Branch != "dev" || !beta.ShowMetadata {
		t.Fatalf("beta tree item = %#v found=%v, want inactive metadata shown by default", beta, found)
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

	beta, found := findSidebarTreeSession(items, "beta")
	if !found {
		t.Fatalf("beta row not found in %#v", items)
	}
	if beta.Session.Metadata.Branch != "dev" || !beta.ShowMetadata {
		t.Fatalf("beta tree item = %#v, want dev metadata shown when inactive metadata enabled", beta)
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

	visible := []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}, {Name: "gamma", Visible: true}}
	target, err := contextualQuickSwitchTarget(ctx, state, visible, visible, 2)
	if err != nil {
		t.Fatalf("contextualQuickSwitchTarget error: %v", err)
	}
	if target != "gamma" {
		t.Fatalf("target = %q, want gamma from current category", target)
	}
}

func TestContextualQuickSwitchTargetIncludesNumericSessionsWhenShown(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  display-message) printf 'alpha\n' ;;
esac
`)
	ctx := context.Background()
	state := ports.PersistedState{Sidebar: &ports.SidebarState{ShowNumericSessions: true}, SessionOrder: []string{"alpha", "3", "beta"}, SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:default", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:default", Name: "Default", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "3"}, {Name: "beta"}}}},
	}}}
	visible := []sessions.View{{Name: "alpha", Visible: true}, {Name: "3", Visible: true}, {Name: "beta", Visible: true}}

	target, err := contextualQuickSwitchTarget(ctx, state, visible, visible, 2)
	if err != nil {
		t.Fatalf("contextualQuickSwitchTarget error: %v", err)
	}
	if target != "3" {
		t.Fatalf("target = %q, want numeric session in visible slot 2", target)
	}
}

func TestContextualQuickSwitchTargetUsesNumericCurrentForActiveCategoryWhenHidden(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  display-message) printf '3\n' ;;
esac
`)
	ctx := context.Background()
	state := ports.PersistedState{SessionOrder: []string{"alpha", "3", "beta"}, SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
		{ID: "category:nums", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:nums", Name: "Nums", Sessions: []ports.SidebarLayoutSessionRef{{Name: "3"}, {Name: "beta"}}}},
	}}}
	layoutVisible := []sessions.View{{Name: "alpha", Visible: true}, {Name: "3", Visible: true}, {Name: "beta", Visible: true}}
	fallbackVisible := []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}}

	target, err := contextualQuickSwitchTarget(ctx, state, layoutVisible, fallbackVisible, 1)
	if err != nil {
		t.Fatalf("contextualQuickSwitchTarget error: %v", err)
	}
	if target != "beta" {
		t.Fatalf("target = %q, want beta from numeric current category visible slot", target)
	}
}

func TestContextualQuickSwitchTargetSkipsCollapsedActiveCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  display-message) printf 'alpha\n' ;;
esac
`)
	ctx := context.Background()
	state := ports.PersistedState{SessionOrder: []string{"alpha", "beta"}, SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Collapsed: true, Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
	}}}
	visible := []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}}

	if target, err := contextualQuickSwitchTarget(ctx, state, visible, visible, 1); err == nil || target != "" {
		t.Fatalf("contextualQuickSwitchTarget = %q, %v; want no visible slot in collapsed category", target, err)
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

func TestSaveNewSidebarCategoryUsesUniqueIDsForDuplicateNames(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveNewSidebarCategory(ctx, "Toto", live); err != nil {
		t.Fatalf("first saveNewSidebarCategory error: %v", err)
	}
	if err := saveNewSidebarCategory(ctx, "Toto", live); err != nil {
		t.Fatalf("second saveNewSidebarCategory error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 3 || layout.Items[1].Category.ID == layout.Items[2].Category.ID || layout.Items[1].Category.Name != "Toto" || layout.Items[2].Category.Name != "Toto" {
		t.Fatalf("duplicate category layout = %#v, want duplicate names with unique ids", layout.Items)
	}
}

func TestSaveSidebarSessionCategoryMovesSessionIntoTargetCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha", "beta"}
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
		{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}},
		{ID: "category:db", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:db", Name: "Databases", Sessions: []ports.SidebarLayoutSessionRef{{Name: "beta"}}}},
	}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := saveSidebarSessionCategory(ctx, "alpha", "category:db", live); err != nil {
		t.Fatalf("saveSidebarSessionCategory error: %v", err)
	}
	state, err = store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items[0].Category.Sessions) != 0 || sessionNamesFromRefs(layout.Items[1].Category.Sessions)[1] != "alpha" {
		t.Fatalf("layout after session move = %#v", layout.Items)
	}
}

func TestSaveSidebarCategoryColorPersistsState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, sidebarlayout.DefaultCategoryID, "#38bdf8", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || layout.Items[0].Category.Color != "#38bdf8" {
		t.Fatalf("layout color = %#v, want default #38bdf8", layout.Items)
	}
}

func TestSaveSidebarCategoryColorClearsStoredColorOnReset(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, sidebarlayout.DefaultCategoryID, "#38bdf8", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor seed error: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, sidebarlayout.DefaultCategoryID, "", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor reset error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || layout.Items[0].Category.Color != "" {
		t.Fatalf("layout color after reset = %#v, want cleared default category color", layout.Items)
	}
}

func TestSaveSidebarCategoryColorTrimsWhitespaceReset(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, sidebarlayout.DefaultCategoryID, "#38bdf8", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor seed error: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, sidebarlayout.DefaultCategoryID, "   ", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor whitespace reset error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || layout.Items[0].Category.Color != "" {
		t.Fatalf("layout color after whitespace reset = %#v, want cleared default category color", layout.Items)
	}
}

func TestSaveSidebarCategoryColorResetIgnoresMissingCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	before, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, "category:missing", "", live); err != nil {
		t.Fatalf("saveSidebarCategoryColor missing reset error = %v, want nil", err)
	}
	after, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if !reflect.DeepEqual(after.SidebarLayout, before.SidebarLayout) {
		t.Fatalf("layout changed after missing reset: got %#v want %#v", after.SidebarLayout, before.SidebarLayout)
	}
}

func TestSaveSidebarCategoryColorRejectsMissingCategory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	before, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := saveSidebarCategoryColor(ctx, "category:missing", "#38bdf8", live); err == nil {
		t.Fatal("saveSidebarCategoryColor missing category error = nil, want error")
	}
	after, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if !reflect.DeepEqual(after.SidebarLayout, before.SidebarLayout) {
		t.Fatalf("layout changed after missing category: got %#v want %#v", after.SidebarLayout, before.SidebarLayout)
	}
}

func TestSaveSidebarCategoryCollapsedPersistsState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveSidebarCategoryCollapsed(ctx, sidebarlayout.DefaultCategoryID, true, live); err != nil {
		t.Fatalf("saveSidebarCategoryCollapsed error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || !layout.Items[0].Category.Collapsed {
		t.Fatalf("layout collapsed = %#v, want default collapsed", layout.Items)
	}
}

func TestSaveDeletedSidebarLayoutItemRemovesSeparatorAndSpacerAndPreservesSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	if err := saveReconciledSidebarLayout(ctx, live); err != nil {
		t.Fatalf("saveReconciledSidebarLayout error: %v", err)
	}
	if err := saveNewSidebarSeparator(ctx, live); err != nil {
		t.Fatalf("saveNewSidebarSeparator error: %v", err)
	}
	if err := saveNewSidebarSpacer(ctx, live); err != nil {
		t.Fatalf("saveNewSidebarSpacer error: %v", err)
	}
	if err := saveDeletedSidebarLayoutItem(ctx, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSeparator, ItemID: "separator:1"}, live); err != nil {
		t.Fatalf("saveDeletedSidebarLayoutItem separator error: %v", err)
	}
	if err := saveDeletedSidebarLayoutItem(ctx, sidebarlayout.Selection{Kind: sidebarlayout.RowKindSpacer, ItemID: "spacer:1"}, live); err != nil {
		t.Fatalf("saveDeletedSidebarLayoutItem spacer error: %v", err)
	}
	state, err := sessionOrderStore().Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || layout.Items[0].Kind != sidebarlayout.ItemKindCategory || len(layout.Items[0].Category.Sessions) != 1 || layout.Items[0].Category.Sessions[0].Name != "alpha" {
		t.Fatalf("layout after delete = %#v, want default category with alpha only", layout.Items)
	}
}

func TestSaveDeletedSidebarCategoryMovesSessionsToDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	live := []string{"alpha"}
	store := sessionOrderStore()
	state, err := store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.SidebarLayout = &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{{ID: "category:work", Kind: string(sidebarlayout.ItemKindCategory), Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}}}}}}
	if err := store.Save(ctx, "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := saveDeletedSidebarLayoutItem(ctx, sidebarlayout.Selection{Kind: sidebarlayout.RowKindCategory, CategoryID: "category:work"}, live); err != nil {
		t.Fatalf("saveDeletedSidebarLayoutItem error: %v", err)
	}
	state, err = store.Load(ctx, "tmux")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	layout := coreLayoutFromPersisted(state.SidebarLayout)
	if len(layout.Items) != 1 || layout.Items[0].Category.ID != sidebarlayout.DefaultCategoryID || len(layout.Items[0].Category.Sessions) != 1 || layout.Items[0].Category.Sessions[0].Name != "alpha" {
		t.Fatalf("layout after category delete = %#v, want default category with alpha", layout.Items)
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

func sidebarTreeFakeTmuxScript(metadataInactive string) string {
	return `#!/usr/bin/env bash
case "$1" in
  show-options)
    if [ "$2" = "-g" ] && [ $# -eq 2 ]; then
      printf '@session-sidebar-key M-b\n'
      printf '@session-sidebar-width 30\n'
      printf '@session-sidebar-project-roots \n'
      printf '@session-sidebar-close-after-switch off\n'
      printf '@session-sidebar-heat-colors on\n'
      printf '@session-sidebar-heat-half-life-hours 8\n'
      printf '@session-sidebar-heat-stale-hours 24\n'
      printf '@session-sidebar-heat-refresh-seconds 60\n'
      printf '@session-sidebar-heat-recent 1h\n'
      printf '@session-sidebar-heat-max-highlighted 0\n'
      printf '@session-sidebar-activity-debug-log off\n'
      printf '@session-sidebar-agent-attention off\n'
      printf '@session-sidebar-agent-attention-animation pulse\n'
      printf '@session-sidebar-auto-sort-recent off\n'
      printf '@session-sidebar-restore-sessions auto\n'
      printf '@session-sidebar-continuum-grace-seconds 3\n'
      printf '@session-sidebar-metadata-subline on\n'
      printf '@session-sidebar-metadata-inactive ` + metadataInactive + `\n'
    else
      case "$3" in
        @session-sidebar-key) printf 'M-b\n' ;;
        @session-sidebar-width) printf '30\n' ;;
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
      esac
    fi ;;
  display-message) printf 'alpha\n' ;;
  list-sessions) printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n' ;;
  list-clients) ;;
esac
`
}
