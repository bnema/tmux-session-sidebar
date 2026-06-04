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

func reloadTestSessions(fn func() []SessionItem) func() []TreeItem {
	return func() []TreeItem {
		items := fn()
		if items == nil {
			return nil
		}
		return testTreeItems(items)
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

func TestSidebarModelBrowseShortcuts(t *testing.T) {
	t.Run("c opens create session menu", func(t *testing.T) {
		model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
		updated, _ := model.Update(keyPress("c", 0))
		model = requireSidebarModel(t, updated)
		if model.mode != ModeCreateSession || !strings.Contains(stripANSI(model.Render()), "Git repo") {
			t.Fatalf("create menu not opened: mode=%s view=%q", model.mode, stripANSI(model.Render()))
		}
	})
	t.Run("n opens layout menu", func(t *testing.T) {
		model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
		updated, _ := model.Update(keyPress("n", 0))
		model = requireSidebarModel(t, updated)
		if model.mode != ModeNewItem || !strings.Contains(stripANSI(model.Render()), "New category") {
			t.Fatalf("layout menu not opened: mode=%s view=%q", model.mode, stripANSI(model.Render()))
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
}

func TestSidebarModelSwitchSelectedReloadsAndKeepsPreviousCurrentSelected(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { return name == "beta" },
		ReloadTreeItems: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Attention: true, Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	model.cursor = 2
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if cmd == nil {
		t.Fatal("switch reload did not schedule attention animation tick")
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after switch reload = %#v ok=%v, want previous alpha", item, ok)
	}
}

func TestSidebarModelKillRequiresConfirmationAndReloadsTree(t *testing.T) {
	called := 0
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession:     func(name string) bool { called++; return name == "alpha" },
		ReloadTreeItems: reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "beta"}} }),
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

func TestSidebarModelSlotSwitchesVisibleSession(t *testing.T) {
	called := ""
	model := newTestSidebarModel([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { called = name; return true },
		ReloadTreeItems: reloadTestSessions(func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		}),
	})
	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)
	if called != "beta" {
		t.Fatalf("SwitchSession called with %q, want beta", called)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after slot switch = %#v ok=%v, want previous alpha", item, ok)
	}
}

func TestSidebarModelReordersSelectedTreeItem(t *testing.T) {
	var gotID string
	var gotDelta int
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{
		MoveTreeItem:    func(id string, delta int) bool { gotID, gotDelta = id, delta; return true },
		ReloadTreeItems: reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "beta"}, {Name: "alpha"}} }),
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

func TestSidebarModelPinColorFlow(t *testing.T) {
	var pinnedName, pinnedColor string
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{PinSessionWithColor: func(name string, color string) bool {
		pinnedName, pinnedColor = name, color
		return true
	}, ReloadTreeItems: reloadTestSessions(func() []SessionItem { return []SessionItem{{Name: "alpha", Pinned: true}} })})
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}))
	model = requireSidebarModel(t, updated)
	if model.mode != ModePinColor || model.pinColorSession != "alpha" {
		t.Fatalf("pin color state = mode=%s session=%q", model.mode, model.pinColorSession)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)
	if pinnedName != "alpha" || pinnedColor == "" || model.mode != ModeBrowse {
		t.Fatalf("pin result name=%q color=%q mode=%s", pinnedName, pinnedColor, model.mode)
	}
}

func TestSidebarModelHelpToggleOpensBottomSheetCheatSheet(t *testing.T) {
	model := newTestSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	view := stripANSI(model.Render())
	for _, want := range []string{"keys", "navigation", "↵ switch", "c create", "n layout", spaceKeySymbol + " pin", "h nums", "J/K", "r rename", "x kill", "esc close"} {
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
