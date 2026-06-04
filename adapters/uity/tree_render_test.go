package uity

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

func TestTreeSidebarRenderUsesCompactSlotsTreeGuidesAndAttentionRight(t *testing.T) {
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha", Attention: true}, Slot: 1, Depth: 1},
		{Kind: TreeRowSession, ID: "category:work/session:beta", CategoryID: "category:work", Session: SessionItem{Name: "beta"}, Slot: 2, Depth: 1, LastChild: true},
	}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	model.attentionAnimationFrame = 1

	view := stripANSI(model.Render())
	if !strings.Contains(view, "▾ Work") || !strings.Contains(view, "├─ 1 "+inactiveMarkerSymbol+" alpha "+attentionMarkerSymbol) || !strings.Contains(view, "└─ 2 "+inactiveMarkerSymbol+" beta") {
		t.Fatalf("tree render missing compact slots, guides, or right attention marker: %q", view)
	}
	if strings.Contains(view, "[1]") || strings.Contains(view, "[2]") {
		t.Fatalf("tree render should not use bracketed slots: %q", view)
	}
}

func TestTreeSidebarReloadsTreeAfterNewItem(t *testing.T) {
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{
		CreateSpacer: func() bool { return true },
		ReloadTreeItems: func() []TreeItem {
			reloaded = true
			return []TreeItem{{Kind: TreeRowSpacer, ID: "spacer:1"}}
		},
	}, SidebarOptions{})

	updated, _ := model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
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

	updated, _ := model.Update(keyPress("h", 0))
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

func TestTreeSidebarNOpensNewItemMenu(t *testing.T) {
	created := ""
	model := NewTreeSidebarModelWithOptions([]TreeItem{
		{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true},
		{Kind: TreeRowSession, ID: "category:work/session:alpha", CategoryID: "category:work", Session: SessionItem{Name: "alpha"}, Depth: 1, LastChild: true},
	}, Actions{CreateCategory: func(name string) bool {
		created = name
		return true
	}}, SidebarOptions{})

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	if !strings.Contains(view, "new layout item") || !strings.Contains(view, "New category") || !strings.Contains(view, "New spacer") || !strings.Contains(view, "New separator") {
		t.Fatalf("new item menu missing bottom sheet choices: %q", view)
	}
	if !strings.Contains(view, "alpha") {
		t.Fatalf("bottom sheet should preserve sidebar content behind menu: %q", view)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if created != "New category" || model.mode != ModeBrowse {
		t.Fatalf("created=%q mode=%s, want default category creation and browse", created, model.mode)
	}
}

func TestTreeSidebarCOpensCreateSessionSheetAndRunsGitChoice(t *testing.T) {
	called := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateGitProject: func() bool {
		called = true
		return true
	}, ReloadTreeItems: func() []TreeItem {
		return []TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}
	}}, SidebarOptions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	model = requireSidebarModel(t, updated)

	updated, _ = model.Update(keyPress("c", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	if !strings.Contains(view, "create session") || !strings.Contains(view, "Git repo") || !strings.Contains(view, "Current dir") || !strings.Contains(view, "Named") {
		t.Fatalf("create session sheet missing choices: %q", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if !called || model.mode != ModeBrowse {
		t.Fatalf("git choice called=%v mode=%s, want called and browse", called, model.mode)
	}
}

func TestTreeSidebarCreateSessionNamedPrompt(t *testing.T) {
	created := ""
	reloaded := false
	model := NewTreeSidebarModelWithOptions([]TreeItem{{Kind: TreeRowCategory, ID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true}}, Actions{CreateNamedSession: func(name string) bool {
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
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("j", 0))
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
	if !strings.Contains(view, "└─ 1 "+inactiveMarkerSymbol+" alpha") || !strings.Contains(view, "    feature/category-tree  2") {
		t.Fatalf("tree render missing session metadata child: %q", view)
	}
}
