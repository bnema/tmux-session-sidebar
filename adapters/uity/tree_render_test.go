package uity

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

func TestTreeSidebarRenderUsesCompactSlotsTreeGuidesAndAttentionRight(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha", Current: true, Attention: true}, Slot: 1, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:beta", CategoryID: "category:work", Session: SessionItem{Name: "beta"}, Slot: 2, Depth: 1, LastChild: true},
	}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	model.attentionAnimationFrame = 1

	view := stripANSI(model.Render())
	if !strings.Contains(view, "▾ Work") || !strings.Contains(view, "├─ 1 alpha "+attentionMarkerSymbol) || !strings.Contains(view, "└─ 2 beta") {
		t.Fatalf("tree render missing compact slots, guides, or right attention marker: %q", view)
	}
	if strings.Contains(view, "[1]") || strings.Contains(view, "[2]") {
		t.Fatalf("tree render should not use bracketed slots: %q", view)
	}
}

func TestTreeSidebarRenderDisplaysContinuousDoubleDigitSlots(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:kappa", CategoryID: "category:work", Session: SessionItem{Name: "kappa"}, Slot: 10, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:lambda", CategoryID: "category:work", Session: SessionItem{Name: "lambda"}, Slot: 11, Depth: 1, LastChild: true},
	}, Actions{}, SidebarOptions{})
	view := stripANSI(model.Render())
	if !strings.Contains(view, "├─ 10 kappa") || !strings.Contains(view, "└─ 11 lambda") {
		t.Fatalf("tree render missing continuous double-digit slots: %q", view)
	}
	if strings.Contains(view, " 0 ") {
		t.Fatalf("tree render should not display slot 10 as 0: %q", view)
	}
}

func TestTreeSidebarSeparatorUsesRendererWidth(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowSeparator, ID: "separator:1"}}, Actions{}, SidebarOptions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 12, Height: 8})
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	if !strings.Contains(view, strings.Repeat("─", 12)) || strings.Contains(view, strings.Repeat("─", 24)) {
		t.Fatalf("separator should fit renderer width, view=%q", view)
	}
}

func TestTreeSidebarScrollsOverflowToKeepSelectionVisible(t *testing.T) {
	items := make([]SessionItem, 0, 8)
	for i := 1; i <= 8; i++ {
		items = append(items, SessionItem{Name: fmt.Sprintf("session-%02d", i), Slot: i})
	}
	model := newTestSidebarModel(items, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	model = requireSidebarModel(t, updated)
	for range 5 {
		updated, _ = model.Update(keyPress("j", 0))
		model = requireSidebarModel(t, updated)
	}

	view := stripANSI(model.Render())
	if strings.Contains(view, "session-01") || strings.Contains(view, "session-02") {
		t.Fatalf("scrolled view should clip early sessions: %q", view)
	}
	if !strings.Contains(view, "session-06") {
		t.Fatalf("scrolled view should keep selected session visible: %q", view)
	}
	if got := len(strings.Split(view, "\n")); got != 6 {
		t.Fatalf("rendered height = %d, want 6: %q", got, view)
	}
}

func TestTreeSidebarScrollAccountingMatchesSuppressedMetadata(t *testing.T) {
	items := make([]SessionItem, 0, 8)
	for i := 1; i <= 8; i++ {
		items = append(items, SessionItem{Name: fmt.Sprintf("session-%02d", i), Slot: i, Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Clean: true}})
	}
	model := newTestSidebarModel(items, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	model = requireSidebarModel(t, updated)
	for range 5 {
		updated, _ = model.Update(keyPress("j", 0))
		model = requireSidebarModel(t, updated)
	}

	view := stripANSI(model.Render())
	if !strings.Contains(view, "session-06") {
		t.Fatalf("selected session should remain visible when metadata is suppressed: %q", view)
	}
}

func TestTreeSidebarTinyHeightsDoNotOverflow(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{})
	for _, height := range []int{1, 2} {
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: height})
		model = requireSidebarModel(t, updated)
		view := stripANSI(model.Render())
		if got := len(strings.Split(view, "\n")); got != height {
			t.Fatalf("height %d rendered %d lines: %q", height, got, view)
		}
	}
}

func TestTreeSidebarPageNavigationClampsInsteadOfWrapping(t *testing.T) {
	items := make([]SessionItem, 0, 8)
	for i := 1; i <= 8; i++ {
		items = append(items, SessionItem{Name: fmt.Sprintf("session-%02d", i), Slot: i})
	}
	model := newTestSidebarModel(items, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "session-08" {
		t.Fatalf("page down selected = %#v ok=%v, want last session", item, ok)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedTreeItem(); !ok || item.ID != "category:default" {
		t.Fatalf("page up selected = %#v ok=%v, want first category", item, ok)
	}
}

func TestTreeSidebarReloadsTreeAfterCreateSpacer(t *testing.T) {
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{
		CreateSpacer: func() bool { return true },
		ReloadTreeItems: func() []TreeItem {
			reloaded = true
			return []TreeItem{{Kind: TreeRowSpacer, ID: "spacer:1"}}
		},
	}, SidebarOptions{})

	updated, _ := model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	for range 6 {
		updated, _ = model.Update(keyPress("j", 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if !reloaded || len(model.treeItems) != 1 || model.treeItems[0].Kind != TreeRowSpacer {
		t.Fatalf("tree reload after new spacer: reloaded=%v tree=%#v", reloaded, model.treeItems)
	}
}

func TestTreeSidebarCanSelectAndRenderCategorySelection(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
		{Kind: TreeRowSeparator, ID: "separator:1"},
	}, Actions{}, SidebarOptions{})

	view := model.Render()
	if !strings.Contains(stripANSI(view), "▾ Work") || !strings.Contains(view, "48;2;6;95;70") {
		t.Fatalf("initial category selection not rendered selected: %q", view)
	}
	updated, _ := model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedTreeItem(); !ok || item.Kind != TreeRowSession {
		t.Fatalf("after j selected tree item = %#v ok=%v, want session", item, ok)
	}
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedTreeItem(); !ok || item.Kind != TreeRowSeparator {
		t.Fatalf("after second j selected tree item = %#v ok=%v, want separator", item, ok)
	}
}

func TestTreeSidebarSessionActionsIgnoreNonSessionSelection(t *testing.T) {
	switched := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{SwitchSession: func(string) bool {
		switched = true
		return true
	}}, SidebarOptions{})

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	_ = requireSidebarModel(t, updated)
	if switched {
		t.Fatal("enter on category called SwitchSession")
	}
}

func TestTreeSidebarMoveReselectsSessionAfterCategoryChangesID(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
	}, Actions{
		MoveTreeItem: func(string, int) bool { return true },
		ReloadTreeItems: func() []TreeItem {
			return []TreeItem{
				{Kind: TreeRowCategory, ID: "category:other", CategoryID: "category:other", CategoryName: "Other", CategoryOpen: true},
				{Kind: TreeRowSession, ID: "category:other/session:alpha", CategoryID: "category:other", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
			}
		},
	}, SidebarOptions{})
	updated, _ := model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)

	updated, _ = model.Update(keyPress("J", tea.ModShift))
	model = requireSidebarModel(t, updated)

	if item, ok := model.selectedTreeItem(); !ok || item.Kind != TreeRowSession || item.Session.Name != "alpha" {
		t.Fatalf("selected after move = %#v ok=%v, want moved alpha session", item, ok)
	}
}

func TestTreeSidebarJMovesSelectedTreeItemAndReloads(t *testing.T) {
	movedID := ""
	delta := 0
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
	}, Actions{
		MoveTreeItem: func(id string, d int) bool { movedID, delta = id, d; return true },
		ReloadTreeItems: func() []TreeItem {
			return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}
		},
	}, SidebarOptions{})

	updated, _ := model.Update(keyPress("J", tea.ModShift))
	model = requireSidebarModel(t, updated)

	if movedID != "category:work" || delta != 1 || len(model.treeItems) != 1 {
		t.Fatalf("move tree item id=%q delta=%d tree=%#v", movedID, delta, model.treeItems)
	}
}

func TestTreeSidebarToggleNumericReloadsTree(t *testing.T) {
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{
		SetShowNumericItems: func(bool) bool { return true },
		ReloadTreeItems: func() []TreeItem {
			reloaded = true
			return []TreeItem{{Kind: TreeRowSession, ID: "category:work/session:1", CategoryID: "category:work", Session: SessionItem{Name: "1"}, Depth: 1, LastChild: true}}
		},
	}, SidebarOptions{})

	updated, _ := model.Update(keyPress("h", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	if !reloaded || len(model.treeItems) != 1 || model.treeItems[0].Session.Name != "1" {
		t.Fatalf("toggle numeric reload: reloaded=%v tree=%#v", reloaded, model.treeItems)
	}
}

func TestTreeSidebarNumericSlotSwitchesOnlyTreeSlot(t *testing.T) {
	switched := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Slot: 1, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:beta", CategoryID: "category:work", Session: SessionItem{Name: "beta"}, Slot: 2, Depth: 1, LastChild: true},
		{Kind: TreeRowCategory, ID: "category:other", CategoryID: "category:other", CategoryName: "Other", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:other/session:gamma", CategoryID: "category:other", Session: SessionItem{Name: "gamma"}, Depth: 1, LastChild: true},
	}, Actions{SwitchSession: func(name string) bool { switched = name; return true }}, SidebarOptions{})

	updated, _ := model.Update(keyPress("2", 0))
	_ = requireSidebarModel(t, updated)
	if switched != "beta" {
		t.Fatalf("switched = %q, want beta", switched)
	}
}

func TestTreeSidebarRPromptsAndRenamesSelectedCategory(t *testing.T) {
	renamedID := ""
	renamedName := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{RenameCategory: func(id string, name string) bool {
		renamedID = id
		renamedName = name
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Client", CategoryOpen: true}}
	}}, SidebarOptions{})

	updated, _ := model.Update(keyPress("r", 0))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeRenameCategory || model.renameCategoryInput != "Work" {
		t.Fatalf("rename mode=%s input=%q, want prompt with Work", model.mode, model.renameCategoryInput)
	}
	for range len("Work") {
		updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		model = requireSidebarModel(t, updated)
	}
	for _, r := range "Client" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if renamedID != "category:work" || renamedName != "Client" || model.mode != ModeBrowse {
		t.Fatalf("renamedID=%q renamedName=%q mode=%s, want category rename", renamedID, renamedName, model.mode)
	}
}

func TestTreeSidebarFilterNoMatchShowsNoSessions(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
	}, Actions{}, SidebarOptions{})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("z", 0))
	model = requireSidebarModel(t, updated)

	view := stripANSI(model.Render())
	if !strings.Contains(view, "no sessions") || strings.Contains(view, "▾ Work") {
		t.Fatalf("filter no-match view = %q, want no sessions without structure", view)
	}
}

func TestTreeSidebarNOpensQuickNamedSessionPrompt(t *testing.T) {
	created := ""
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
	}, Actions{CreateNamedSession: func(name string, categoryID string) bool {
		created = name
		return true
	}, ReloadTreeItems: func() []TreeItem {
		reloaded = true
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}
	}}, SidebarOptions{})

	updated, _ := model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeCreateNamed || !strings.Contains(stripANSI(model.Render()), "new session") {
		t.Fatalf("n did not open quick named session prompt: mode=%s view=%q", model.mode, stripANSI(model.Render()))
	}
	for _, key := range []string{"s", "c", "r", "a", "t", "c", "h"} {
		updated, _ = model.Update(keyPress(key, 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if created != "scratch" || !reloaded || model.mode != ModeBrowse {
		t.Fatalf("created=%q reloaded=%v mode=%s, want scratch reload browse", created, reloaded, model.mode)
	}
}

func TestTreeSidebarCOpensCreateSessionSheetAndRunsGitChoice(t *testing.T) {
	called := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateGitProject: func(categoryID string) bool {
		called = true
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}
	}}, SidebarOptions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 16})
	model = requireSidebarModel(t, updated)

	updated, _ = model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	for _, want := range []string{"CREATE MENU", "SESSIONS", "new named session", "from git repo", "from pwd", "from project picker", "LAYOUT", "category", "separator", "empty space"} {
		if !strings.Contains(view, want) {
			t.Fatalf("create sheet missing %q: %q", want, view)
		}
	}
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if !called || model.mode != ModeBrowse {
		t.Fatalf("git choice called=%v mode=%s, want called and browse", called, model.mode)
	}
}

func TestTreeSidebarCreateMenuUsesSelectedSessionCategory(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "git repo", keys: []string{"j"}, want: "git"},
		{name: "pwd", keys: []string{"j", "j"}, want: "pwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAction, gotCategoryID string
			model := NewTreeSidebarModelWithOptions([]TreeItem{
				{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
				{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Slot: 1, Depth: 1},
			}, Actions{
				CreateGitProject: func(categoryID string) bool {
					gotAction, gotCategoryID = "git", categoryID
					return true
				},
				CreateAdhoc: func(categoryID string) bool {
					gotAction, gotCategoryID = "pwd", categoryID
					return true
				},
				ReloadTreeItems: func() []TreeItem { return nil },
			}, SidebarOptions{})
			updated, _ := model.Update(keyPress("j", 0))
			model = requireSidebarModel(t, updated)
			updated, _ = model.Update(keyPress("c", 0))
			model = requireSidebarModel(t, updated)
			for _, key := range tt.keys {
				updated, _ = model.Update(keyPress(key, 0))
				model = requireSidebarModel(t, updated)
			}
			updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			requireSidebarModel(t, updated)
			if gotAction != tt.want || gotCategoryID != "category:work" {
				t.Fatalf("action/category = %q/%q, want %q/category:work", gotAction, gotCategoryID, tt.want)
			}
		})
	}
}

func TestTreeSidebarCreateNamedSessionUsesSelectedSessionCategory(t *testing.T) {
	var gotName, gotCategoryID string
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Slot: 1, Depth: 1},
	}, Actions{CreateNamedSession: func(name string, categoryID string) bool {
		gotName = name
		gotCategoryID = categoryID
		return true
	}, ReloadTreeItems: func() []TreeItem { return nil }}, SidebarOptions{})
	updated, _ := model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	for _, r := range "scratch" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	requireSidebarModel(t, updated)
	if gotName != "scratch" || gotCategoryID != "category:work" {
		t.Fatalf("CreateNamedSession = %q/%q, want scratch/category:work", gotName, gotCategoryID)
	}
}

func TestTreeSidebarCreateMenuNavigationSkipsGroupHeaders(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{}, SidebarOptions{})
	updated, _ := model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	if model.menu.Cursor != 1 || model.menu.Items[model.menu.Cursor].Label != "new named session" {
		t.Fatalf("initial create cursor = %d/%q, want first selectable new named session", model.menu.Cursor, model.menu.Items[model.menu.Cursor].Label)
	}
	updated, _ = model.Update(keyPress("k", 0))
	model = requireSidebarModel(t, updated)
	if model.menu.Items[model.menu.Cursor].Header || model.menu.Items[model.menu.Cursor].Label != "empty space" {
		t.Fatalf("cursor after wrap-up = %d/%q header=%v, want empty space", model.menu.Cursor, model.menu.Items[model.menu.Cursor].Label, model.menu.Items[model.menu.Cursor].Header)
	}
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	if model.menu.Items[model.menu.Cursor].Header || model.menu.Items[model.menu.Cursor].Label != "new named session" {
		t.Fatalf("cursor after wrap-down = %d/%q header=%v, want new named session", model.menu.Cursor, model.menu.Items[model.menu.Cursor].Label, model.menu.Items[model.menu.Cursor].Header)
	}
}

func TestTreeSidebarCreateNamedSessionUsesSelectedCategory(t *testing.T) {
	var gotName, gotCategoryID string
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateNamedSession: func(name string, categoryID string) bool {
		gotName = name
		gotCategoryID = categoryID
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}
	}}, SidebarOptions{})
	updated, _ := model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	for _, r := range "scratch" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	requireSidebarModel(t, updated)
	if gotName != "scratch" || gotCategoryID != "category:work" {
		t.Fatalf("CreateNamedSession = %q/%q, want scratch/category:work", gotName, gotCategoryID)
	}
}

func TestTreeSidebarCategoryCollapseShortcuts(t *testing.T) {
	var calls []struct {
		categoryID string
		collapsed  bool
	}
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{SetCategoryCollapsed: func(categoryID string, collapsed bool) bool {
		calls = append(calls, struct {
			categoryID string
			collapsed  bool
		}{categoryID: categoryID, collapsed: collapsed})
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: false}}
	}}, SidebarOptions{})
	updated, _ := model.Update(keyPress("h", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("l", 0))
	requireSidebarModel(t, updated)
	if len(calls) != 2 || calls[0].categoryID != "category:work" || !calls[0].collapsed || calls[1].categoryID != "category:work" || calls[1].collapsed {
		t.Fatalf("collapse calls = %#v, want category collapse then expand", calls)
	}
}

func TestTreeSidebarCreateCategoryPromptsForName(t *testing.T) {
	created := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateCategory: func(name string) bool {
		created = name
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:new", CategoryID: "category:new", CategoryName: "Databases", CategoryOpen: true}}
	}}, SidebarOptions{})
	updated, _ := model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	for range 4 {
		updated, _ = model.Update(keyPress("j", 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeCreateCategory {
		t.Fatalf("mode=%s, want create category prompt", model.mode)
	}
	for _, r := range "Databases" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if created != "Databases" || model.mode != ModeBrowse {
		t.Fatalf("created=%q mode=%s, want Databases browse", created, model.mode)
	}
}

func TestTreeSidebarCreateSessionNamedPrompt(t *testing.T) {
	created := ""
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateNamedSession: func(name string, categoryID string) bool {
		created = name
		return true
	}, ReloadTreeItems: func() []TreeItem {
		reloaded = true
		return []TreeItem{
			{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
			{Kind: TreeRowSession, ID: "category:work/session:scratch", CategoryID: "category:work", Session: SessionItem{Name: "scratch", Current: true}, Depth: 1, LastChild: true},
		}
	}}, SidebarOptions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if model.mode != ModeCreateNamed {
		t.Fatalf("mode=%s, want create named", model.mode)
	}
	for _, r := range "scratch" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if created != "scratch" || model.mode != ModeBrowse || !reloaded {
		t.Fatalf("created=%q mode=%s reloaded=%v, want named session and tree reload", created, model.mode, reloaded)
	}
	if item, ok := model.selectedTreeItem(); !ok || item.Session.Name != "scratch" {
		t.Fatalf("selected tree item = %#v, %v; want reloaded scratch session", item, ok)
	}
}

func TestTreeSidebarRenderShowsMetadataAsTreeChild(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha", Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Branch: "feature/category-tree", Modified: 2}}, Slot: 1, Depth: 1, LastChild: true, ShowMetadata: true},
	}, Actions{}, SidebarOptions{})

	view := stripANSI(model.Render())
	if !strings.Contains(view, "└─ 1 alpha") || !strings.Contains(view, "     feature/category-tree  2") {
		t.Fatalf("tree render missing session metadata child: %q", view)
	}
}
