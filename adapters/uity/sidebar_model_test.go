package uity

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

func newTestSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return newTestSidebarModelWithOptions(items, actions, SidebarOptions{})
}

func newTestSidebarModelWithOptions(items []SessionItem, actions Actions, options SidebarOptions) SidebarModel {
	model := NewTreeSidebarModelWithOptions(testTreeItems(items), actions, options)
	if len(items) > 0 {
		model.cursor = 1
	}
	return model
}

func testTreeItems(items []SessionItem) []TreeItem {
	tree := make([]TreeItem, 0, len(items)+1)
	tree = append(tree, TreeItem{Kind: TreeRowCategory, ID: "category:default", CategoryID: "category:default", CategoryName: "Default", CategoryOpen: true})
	for i, item := range items {
		tree = append(tree, TreeItem{Kind: TreeRowSession, ID: "category:default/session:" + item.Name, CategoryID: "category:default", Session: item, Slot: item.Slot, Depth: 1, LastChild: i == len(items)-1, ShowMetadata: item.Metadata.Kind != ""})
	}
	return tree
}

func reloadTestSessions(fn func() []SessionItem) func() *ReloadResult {
	return func() *ReloadResult {
		items := fn()
		if items == nil {
			return nil
		}
		return &ReloadResult{Items: testTreeItems(items), Appearance: config.ColorSchemeAppearanceDark}
	}
}

func TestSidebarModelInitDoesNotSchedulePeriodicRefresh(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	if cmd := model.Init(); cmd != nil {
		t.Fatal("Init() scheduled a periodic refresh command")
	}
}

func TestSidebarModelViewEnablesMouseWheelEvents(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	view := model.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("View().MouseMode = %v, want %v", view.MouseMode, tea.MouseModeCellMotion)
	}
}

func TestSidebarModelMouseWheelNavigatesTreeSessions(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{})
	updated, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" {
		t.Fatalf("selection after wheel down = %#v ok=%v, want beta", item, ok)
	}
}

func TestSidebarModelSearchFiltersTreeSessions(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	if !strings.Contains(view, "beta") || strings.Contains(view, "alpha") || strings.Contains(view, "gamma") {
		t.Fatalf("filtered tree view = %q, want only beta session", view)
	}
}

func TestSidebarModelEscClearsCommittedSearchFilter(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeBrowse || model.filter != "b" {
		t.Fatalf("committed filter state = mode %s filter %q, want browse with active filter", model.mode, model.filter)
	}

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("esc with an active filter should not close the sidebar")
	}
	if model.filter != "" || model.mode != ModeBrowse {
		t.Fatalf("state after esc = mode %s filter %q, want browse with filter cleared", model.mode, model.filter)
	}
	view := stripANSI(model.Render())
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "beta") || !strings.Contains(view, "gamma") {
		t.Fatalf("cleared filter view = %q, want all sessions visible", view)
	}
}

func TestSidebarModelSearchFiltersToMatchingSessionsAndAncestorCategories(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1},
		{Kind: TreeRowSeparator, ID: "separator:1"},
		{Kind: TreeRowCategory, ID: "category:other", CategoryID: "category:other", CategoryName: "Other", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:other/session:beta", CategoryID: "category:other", Session: SessionItem{Name: "beta"}, Depth: 1},
	}, Actions{}, SidebarOptions{})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	for _, r := range "bet" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	view := stripANSI(model.Render())
	if strings.Contains(view, "Work") || strings.Contains(view, "alpha") || strings.Contains(view, "──") {
		t.Fatalf("filter rendered unrelated rows: %q", view)
	}
	if !strings.Contains(view, "Other") || !strings.Contains(view, "beta") {
		t.Fatalf("filter missing matching session and ancestor: %q", view)
	}
	visible := model.selectableTreeItems()
	if len(visible) != 2 || visible[0].ID != "category:other" || visible[1].Session.Name != "beta" {
		t.Fatalf("selectable filtered items = %#v, want other category and beta only", visible)
	}
}

func TestSidebarModelBrowseShortcuts(t *testing.T) {
	t.Run("c opens create menu", func(t *testing.T) {
		model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
		model = requireSidebarModel(t, updated)
		updated, _ = model.Update(keyPress("c", 0))
		model = requireSidebarModel(t, updated)
		view := stripANSI(model.Render())
		for _, want := range []string{"CREATE MENU", "SESSIONS", "new named session", "from git repo", "from pwd", "from project picker", "LAYOUT", "category", "separator", "empty space"} {
			if model.mode != ModeCreate || !strings.Contains(view, want) {
				t.Fatalf("create menu missing %q: mode=%s view=%q", want, model.mode, view)
			}
		}
	})
	t.Run("n opens quick named session prompt", func(t *testing.T) {
		model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
		updated, _ := model.Update(keyPress("n", 0))
		model = requireSidebarModel(t, updated)
		if model.mode != ModeCreateNamed || !strings.Contains(stripANSI(model.Render()), "new session") {
			t.Fatalf("new session prompt not opened: mode=%s view=%q", model.mode, stripANSI(model.Render()))
		}
	})
	t.Run("x starts kill confirmation", func(t *testing.T) {
		model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{KillSession: func(string) bool { return true }})
		model.cursor = 1
		updated, _ := model.Update(keyPress("x", 0))
		model = requireSidebarModel(t, updated)
		if model.mode != ModeConfirmKill || model.pendingKill != "alpha" {
			t.Fatalf("kill confirmation state = mode=%s pending=%q", model.mode, model.pendingKill)
		}
	})
	for _, kind := range []TreeRowKind{TreeRowSeparator, TreeRowSpacer} {
		t.Run("create from "+string(kind)+" keeps nearest category", func(t *testing.T) {
			gotCategoryID := ""
			model := NewTreeSidebarModelWithOptions([]TreeItem{
				{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
				{Kind: kind, ID: "layout:middle"},
				{Kind: TreeRowCategory, ID: "category:other", CategoryID: "category:other", CategoryName: "Other", CategoryOpen: true},
			}, Actions{CreateGitProject: func(categoryID string) bool {
				gotCategoryID = categoryID
				return true
			}}, SidebarOptions{})
			model.cursor = 1
			updated, _ := model.Update(keyPress("g", 0))
			requireSidebarModel(t, updated)
			if gotCategoryID != "category:work" {
				t.Fatalf("CreateGitProject category = %q, want category:work", gotCategoryID)
			}
		})
	}
}

func TestSidebarModelSwitchSelectedReloadsAndKeepsSwitchedSessionSelected(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { return name == "beta" },
		ReloadTree: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Attention: true, Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	model.cursor = 2
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if cmd == nil {
		t.Fatal("switch reload did not schedule attention animation tick")
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" || !item.Current {
		t.Fatalf("selected after switch reload = %#v ok=%v, want switched beta", item, ok)
	}
}

func TestSidebarModelReloadKeepsPreviousCurrentSelectedWhenCurrentChanges(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		ReloadTree: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	})
	model.cursor = 1

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after current change reload = %#v ok=%v, want previous current alpha selected", item, ok)
	}
}

func TestSidebarModelReloadPreservesSelectionWhenPreviousCurrentWasEmpty(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		ReloadTree: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	})
	model.cursor = 1

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after empty-current reload = %#v ok=%v, want preserved alpha", item, ok)
	}
}

func TestSidebarModelReloadPreservesSelectionWhenPreviousCurrentIsFilteredOut(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		ReloadTree: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	})
	model.filter = "bet"
	model.cursor = 1

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" {
		t.Fatalf("selected after filtered previous current reload = %#v ok=%v, want preserved visible beta", item, ok)
	}
}

func TestSidebarModelDeleteSelectedTreeItemRequiresConfirmationAndReloadsTree(t *testing.T) {
	deleted := TreeItem{}
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		DeleteTreeItem: func(item TreeItem) bool { deleted = item; return true },
		ReloadTree:     reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "beta"}} }),
	})
	updated, _ := model.Update(keyPress("d", 0))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeConfirmDelete || model.message != "Delete alpha? y/N" {
		t.Fatalf("delete confirmation state = mode=%s message=%q", model.mode, model.message)
	}
	if view := model.Render(); !strings.Contains(view, "38;2;248;113;113") {
		t.Fatalf("delete confirmation should render destructive red, view=%q", view)
	}
	updated, _ = model.Update(keyPress("y", 0))
	model = requireSidebarModel(t, updated)
	items := sessionItemsFromTree(model.treeItems)
	if deleted.Session.Name != "alpha" || len(items) != 1 || items[0].Name != "beta" || model.mode != ModeBrowse {
		t.Fatalf("deleted=%#v items=%#v mode=%s", deleted, items, model.mode)
	}
}

func TestSidebarModelDeleteSelectedCategory(t *testing.T) {
	deleted := TreeItem{}
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{DeleteTreeItem: func(item TreeItem) bool {
		deleted = item
		return true
	}, ReloadTree: func() *ReloadResult {
		return &ReloadResult{Items: []TreeItem{{Kind: TreeRowCategory, ID: "category:default", CategoryID: "category:default", CategoryName: "Default", CategoryOpen: true}}, Appearance: config.ColorSchemeAppearanceDark}
	}}, SidebarOptions{})
	updated, _ := model.Update(keyPress("d", 0))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeConfirmDelete || model.message != "Delete Work? y/N" {
		t.Fatalf("delete category confirmation = mode=%s message=%q", model.mode, model.message)
	}
	updated, _ = model.Update(keyPress("y", 0))
	model = requireSidebarModel(t, updated)
	if deleted.ID != "category:work" || model.treeItems[0].ID != "category:default" {
		t.Fatalf("deleted=%#v tree=%#v", deleted, model.treeItems)
	}
}

func TestSidebarModelDeleteConfirmationCanBeCancelled(t *testing.T) {
	called := false
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{DeleteTreeItem: func(TreeItem) bool { called = true; return true }})
	updated, _ := model.Update(keyPress("d", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	if called || model.mode != ModeBrowse || model.message != "" {
		t.Fatalf("cancel state called=%v mode=%s message=%q", called, model.mode, model.message)
	}
}

func TestSidebarModelKillRequiresConfirmationAndReloadsTree(t *testing.T) {
	called := 0
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(name string) bool { called++; return name == "alpha" },
		ReloadTree:  reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "beta"}} }),
	})
	updated, _ := model.Update(keyPress("x", 0))
	model = requireSidebarModel(t, updated)
	if called != 0 || model.mode != ModeConfirmKill {
		t.Fatalf("kill before confirmation called=%d mode=%s", called, model.mode)
	}
	updated, _ = model.Update(keyPress("y", 0))
	model = requireSidebarModel(t, updated)
	items := sessionItemsFromTree(model.treeItems)
	if called != 1 || len(items) != 1 || items[0].Name != "beta" {
		t.Fatalf("kill/reload called=%d items=%#v", called, items)
	}
}

func TestSidebarModelConfirmationKeys(t *testing.T) {
	tests := []struct {
		name       string
		startKey   string
		confirmKey tea.KeyPressMsg
		wantCalled bool
		wantMode   Mode
	}{
		{name: "kill lowercase yes", startKey: "x", confirmKey: keyPress("y", 0), wantCalled: true, wantMode: ModeBrowse},
		{name: "kill uppercase yes", startKey: "x", confirmKey: keyPress("Y", 0), wantCalled: true, wantMode: ModeBrowse},
		{name: "kill lowercase no", startKey: "x", confirmKey: keyPress("n", 0), wantMode: ModeBrowse},
		{name: "kill uppercase no", startKey: "x", confirmKey: keyPress("N", 0), wantMode: ModeBrowse},
		{name: "kill enter cancels", startKey: "x", confirmKey: tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), wantMode: ModeBrowse},
		{name: "kill escape cancels", startKey: "x", confirmKey: tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}), wantMode: ModeBrowse},
		{name: "kill unrelated stays pending", startKey: "x", confirmKey: keyPress("q", 0), wantMode: ModeConfirmKill},
		{name: "delete lowercase yes", startKey: "d", confirmKey: keyPress("y", 0), wantCalled: true, wantMode: ModeBrowse},
		{name: "delete uppercase yes", startKey: "d", confirmKey: keyPress("Y", 0), wantCalled: true, wantMode: ModeBrowse},
		{name: "delete lowercase no", startKey: "d", confirmKey: keyPress("n", 0), wantMode: ModeBrowse},
		{name: "delete uppercase no", startKey: "d", confirmKey: keyPress("N", 0), wantMode: ModeBrowse},
		{name: "delete enter cancels", startKey: "d", confirmKey: tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), wantMode: ModeBrowse},
		{name: "delete escape cancels", startKey: "d", confirmKey: tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}), wantMode: ModeBrowse},
		{name: "delete unrelated stays pending", startKey: "d", confirmKey: keyPress("q", 0), wantMode: ModeConfirmDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
				KillSession:    func(string) bool { called = true; return true },
				DeleteTreeItem: func(TreeItem) bool { called = true; return true },
			})
			updated, _ := model.Update(keyPress(tt.startKey, 0))
			model = requireSidebarModel(t, updated)
			updated, _ = model.Update(tt.confirmKey)
			model = requireSidebarModel(t, updated)
			if called != tt.wantCalled || model.mode != tt.wantMode {
				t.Fatalf("called=%v mode=%s, want called=%v mode=%s", called, model.mode, tt.wantCalled, tt.wantMode)
			}
		})
	}
}

func TestSidebarModelSlotSwitchPreservesSelectionWhenReloadFails(t *testing.T) {
	called := ""
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { called = name; return true },
		ReloadTree:    func() *ReloadResult { return nil },
	})
	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)
	if called != "beta" {
		t.Fatalf("SwitchSession called with %q, want beta", called)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after failed reload = %#v ok=%v, want preserved alpha", item, ok)
	}
}

func TestSidebarModelSlotSwitchesVisibleSession(t *testing.T) {
	called := ""
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { called = name; return true },
		ReloadTree: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	})
	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)
	if called != "beta" {
		t.Fatalf("SwitchSession called with %q, want beta", called)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" || !item.Current {
		t.Fatalf("selected after slot switch = %#v ok=%v, want switched beta", item, ok)
	}
}

func TestSidebarModelSlotSwitchIgnoresHiddenNumberedSession(t *testing.T) {
	called := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha", Current: true, Slot: 1}, Slot: 1, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:hidden", CategoryID: "category:work", Session: SessionItem{Name: "hidden", Slot: 2}, Slot: 2, Depth: 1, OverflowHidden: true},
	}, Actions{SwitchSession: func(name string) bool {
		called = name
		return true
	}}, SidebarOptions{})
	model.cursor = 1

	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)
	if called != "" {
		t.Fatalf("SwitchSession called with hidden slot %q, want no switch", called)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after hidden slot = %#v ok=%v, want alpha", item, ok)
	}
}

func TestSidebarModelSlotSwitchAllowsNumberedSessionVisibleInFilter(t *testing.T) {
	called := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha", Current: true, Slot: 1}, Slot: 1, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:hidden", CategoryID: "category:work", Session: SessionItem{Name: "hidden", Slot: 2}, Slot: 2, Depth: 1, OverflowHidden: true},
	}, Actions{SwitchSession: func(name string) bool {
		called = name
		return true
	}}, SidebarOptions{})
	model.filter = "hid"

	updated, _ := model.Update(keyPress("2", 0))
	requireSidebarModel(t, updated)
	if called != "hidden" {
		t.Fatalf("SwitchSession called with %q, want visible filtered session hidden", called)
	}
}

func TestSidebarModelReordersSelectedTreeItem(t *testing.T) {
	var gotID string
	var gotDelta int
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{
		MoveTreeItem: func(id string, delta int) bool { gotID, gotDelta = id, delta; return true },
		ReloadTree:   reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "beta"}, {Name: "alpha"}} }),
	})
	updated, _ := model.Update(keyPress("J", tea.ModShift))
	model = requireSidebarModel(t, updated)
	if gotID != "category:default/session:alpha" || gotDelta != 1 {
		t.Fatalf("MoveTreeItem = %q/%d, want alpha id/+1", gotID, gotDelta)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after reorder = %#v ok=%v, want alpha", item, ok)
	}
}

func TestSidebarModelSpaceTogglesPinWithoutOpeningColorPicker(t *testing.T) {
	var pinnedName string
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{TogglePinnedSession: func(name string) bool {
		pinnedName = name
		return true
	}, ReloadTree: reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "alpha", Pinned: true}} })})

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}))
	model = requireSidebarModel(t, updated)

	if pinnedName != "alpha" || model.mode != ModeBrowse {
		t.Fatalf("space pin result name=%q mode=%s, want alpha browse", pinnedName, model.mode)
	}
}

func TestSidebarModelColorizesSelectedSessionWithC(t *testing.T) {
	var coloredName, coloredColor string
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{ColorSession: func(name string, color string) bool {
		coloredName, coloredColor = name, color
		return true
	}, ReloadTree: reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "alpha", PinColor: "#38bdf8"}} })})

	updated, _ := model.Update(keyPress("C", tea.ModShift))
	model = requireSidebarModel(t, updated)
	if model.mode != ModePinColor || model.colorTarget.SessionName != "alpha" {
		t.Fatalf("color picker state = mode=%s target=%#v", model.mode, model.colorTarget)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if coloredName != "alpha" || coloredColor == "" || model.mode != ModeBrowse {
		t.Fatalf("color result name=%q color=%q mode=%s", coloredName, coloredColor, model.mode)
	}
}

func TestSidebarModelColorizesSelectedCategoryWithC(t *testing.T) {
	var categoryID, categoryColor string
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{ColorCategory: func(id string, color string) bool {
		categoryID, categoryColor = id, color
		return true
	}, ReloadTree: func() *ReloadResult {
		return &ReloadResult{Items: []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true, Color: "#38bdf8"}}, Appearance: config.ColorSchemeAppearanceDark}
	}}, SidebarOptions{})

	updated, _ := model.Update(keyPress("C", tea.ModShift))
	model = requireSidebarModel(t, updated)
	if model.mode != ModePinColor || model.colorTarget.CategoryID != "category:work" {
		t.Fatalf("category color picker state = mode=%s target=%#v", model.mode, model.colorTarget)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if categoryID != "category:work" || categoryColor == "" || model.mode != ModeBrowse {
		t.Fatalf("category color result id=%q color=%q mode=%s", categoryID, categoryColor, model.mode)
	}
	if view := model.Render(); !strings.Contains(view, "38;2;56;189;248") {
		t.Fatalf("category color did not render in view=%q", view)
	}
}

func TestSidebarModelColorPickerPositionsBelowSelectedItem(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}, {Name: "delta"}}, Actions{ColorSession: func(string, string) bool { return true }})
	model.width = 50
	model.height = 20
	model.cursor = 3

	updated, _ := model.Update(keyPress("C", tea.ModShift))
	model = requireSidebarModel(t, updated)
	lines := strings.Split(stripANSI(model.Render()), "\n")
	selectedLine := -1
	pickerLine := -1
	for i, line := range lines {
		if strings.Contains(line, "gamma") {
			selectedLine = i
		}
		if strings.Contains(line, "color") {
			pickerLine = i
			break
		}
	}
	if selectedLine < 0 || pickerLine <= selectedLine {
		t.Fatalf("picker line=%d selected line=%d lines=%q, want picker below selected", pickerLine, selectedLine, lines)
	}
}

func TestSessionRowColorPreservesCurrentEmphasis(t *testing.T) {
	style := sessionRowStyle(newSidebarStyles(), SessionItem{Name: "alpha", Current: true, PinColor: "#38bdf8"})
	if !style.GetBold() {
		t.Fatal("colored current session style should preserve active bold emphasis")
	}
}

func TestSidebarModelRenderUsesConfiguredLightAppearancePalette(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Appearance: config.ColorSchemeAppearanceLight})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	model = requireSidebarModel(t, updated)
	view := model.Render()
	if !strings.Contains(view, "48;2;167;243;208") {
		t.Fatalf("render missing light selected background, view=%q", view)
	}
	if strings.Contains(view, "48;2;6;95;70") {
		t.Fatalf("render still used dark selected background, view=%q", view)
	}
}

func TestSidebarModelReloadTreePreservesLightAppearancePalette(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{ReloadTree: func() *ReloadResult {
		return &ReloadResult{Items: testTreeItems([]SessionItem{{Name: "alpha"}}), Appearance: config.ColorSchemeAppearanceLight}
	}}, SidebarOptions{Appearance: config.ColorSchemeAppearanceLight})
	if !model.reloadTreeItems() {
		t.Fatal("reloadTreeItems() = false, want true")
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	model = requireSidebarModel(t, updated)
	view := model.Render()
	if !strings.Contains(view, "48;2;167;243;208") {
		t.Fatalf("render missing light selected background after reload, view=%q", view)
	}
	if strings.Contains(view, "48;2;6;95;70") {
		t.Fatalf("render still used dark selected background after reload, view=%q", view)
	}
}

func TestSidebarModelHelpToggleOpensBottomSheetCheatSheet(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	for _, want := range []string{"keys", "navigation", "↵ switch", "c create", "n new", spaceKeySymbol + " pin", "C color", "alt+h nums", "J/K", "r rename", "d del", "esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help sheet missing %q in %q", want, view)
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = requireSidebarModel(t, updated)
	if model.showHelp {
		t.Fatal("esc should close help sheet")
	}
}

func TestBestEffortMetadataIconModeUsesASCIIForASCIILocale(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	if got := bestEffortMetadataIconMode(); got != MetadataIconsASCII {
		t.Fatalf("bestEffortMetadataIconMode() = %q, want %q", got, MetadataIconsASCII)
	}
}

func keyPress(text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0], Mod: mod})
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func requireSidebarModel(t *testing.T, model tea.Model) SidebarModel {
	t.Helper()
	sidebar, ok := model.(SidebarModel)
	if !ok {
		t.Fatalf("model type = %T, want SidebarModel", model)
	}
	return sidebar
}
