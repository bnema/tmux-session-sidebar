package uity

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/core/heat"
)

func TestSidebarModelInitDoesNotSchedulePeriodicRefresh(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{})
	if cmd := model.Init(); cmd != nil {
		t.Fatal("Init() scheduled a periodic refresh command")
	}
}

func TestSidebarModelF5ReloadsSessionsOnDemand(t *testing.T) {
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		ReloadSessions: func() []SessionItem {
			reloaded++
			return []SessionItem{{Name: "beta"}}
		},
	})

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	model = requireSidebarModel(t, updated)
	if reloaded != 1 {
		t.Fatalf("ReloadSessions called %d times, want 1", reloaded)
	}
	if len(model.items) != 1 || model.items[0].Name != "beta" {
		t.Fatalf("model items after F5 reload = %#v", model.items)
	}
	if cmd != nil {
		t.Fatal("Update(F5) returned an unexpected follow-up command")
	}
}

func TestSidebarModelKillRequiresInlineConfirmation(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(name string) bool {
			called++
			if name != "alpha" {
				t.Fatalf("KillSession called with %q, want alpha", name)
			}
			return true
		},
		ReloadSessions: func() []SessionItem { return []SessionItem{{Name: "beta"}} },
	})

	updated, _ := model.Update(keyPress("x", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	if called != 0 {
		t.Fatalf("KillSession called before confirmation")
	}
	if model.mode != ModeConfirmKill || model.pendingKill != "alpha" {
		t.Fatalf("model confirmation state = mode %q pending %q", model.mode, model.pendingKill)
	}

	updated, _ = model.Update(keyPress("y", 0))
	model = requireSidebarModel(t, updated)
	if called != 1 {
		t.Fatalf("KillSession call count = %d, want 1", called)
	}
	if model.mode != ModeBrowse || model.pendingKill != "" {
		t.Fatalf("model did not clear confirmation: mode %q pending %q", model.mode, model.pendingKill)
	}
	if len(model.items) != 1 || model.items[0].Name != "beta" {
		t.Fatalf("model items after reload = %#v", model.items)
	}
}

func TestSidebarModelKillConfirmationAcceptsUppercaseAndCancelKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     tea.KeyPressMsg
		wantHit bool
	}{
		{name: "uppercase yes", key: keyPress("Y", tea.ModShift), wantHit: true},
		{name: "uppercase no", key: keyPress("N", tea.ModShift)},
		{name: "enter cancels", key: tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})},
		{name: "escape cancels", key: tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := 0
			model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
				KillSession: func(string) bool { called++; return true },
			})
			updated, _ := model.Update(keyPress("x", tea.ModAlt))
			model = requireSidebarModel(t, updated)
			updated, _ = model.Update(tt.key)
			model = requireSidebarModel(t, updated)
			if (called == 1) != tt.wantHit {
				t.Fatalf("KillSession called %d times, wantHit %v", called, tt.wantHit)
			}
			if model.mode != ModeBrowse || model.pendingKill != "" || model.message != "" {
				t.Fatalf("confirmation did not clear: %#v", model)
			}
		})
	}
}

func TestSidebarModelKillConfirmationCanBeCancelled(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "lowercase no", key: keyPress("n", 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := 0
			model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
				KillSession: func(string) bool {
					called++
					return true
				},
			})
			updated, _ := model.Update(keyPress("x", tea.ModAlt))
			model = requireSidebarModel(t, updated)
			updated, _ = model.Update(tt.key)
			model = requireSidebarModel(t, updated)
			if called != 0 {
				t.Fatalf("KillSession called after cancellation")
			}
			if model.mode != ModeBrowse || model.pendingKill != "" || model.message != "" {
				t.Fatalf("model did not clear cancellation: %#v", model)
			}
		})
	}
}

func TestSidebarModelKillConfirmationAllowsCtrlC(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(string) bool { return true },
	})
	updated, _ := model.Update(keyPress("x", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	_, cmd := model.Update(keyPress("c", tea.ModCtrl))
	if cmd == nil {
		t.Fatal("expected ctrl+c to return quit command during kill confirmation")
	}
}

func TestSidebarModelKillConfirmationIgnoresUnrelatedKeys(t *testing.T) {
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(string) bool { return true },
		ReloadSessions: func() []SessionItem {
			reloaded++
			return []SessionItem{{Name: "beta"}}
		},
	})
	updated, _ := model.Update(keyPress("x", tea.ModAlt))
	model = requireSidebarModel(t, updated)

	for _, key := range []tea.KeyPressMsg{
		keyPress("?", tea.ModAlt),
		keyPress("h", tea.ModAlt),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}),
		keyPress("a", 0),
	} {
		updated, _ = model.Update(key)
		model = requireSidebarModel(t, updated)
	}
	if model.mode != ModeConfirmKill || model.pendingKill != "alpha" || model.message == "" {
		t.Fatalf("confirmation state changed after unrelated key: %#v", model)
	}
	if model.showHelp || model.showNumeric || reloaded != 0 {
		t.Fatalf("unrelated key changed state: showHelp=%v showNumeric=%v reloaded=%d", model.showHelp, model.showNumeric, reloaded)
	}
}

func TestSidebarModelAltJReordersSelectedSessionDownAndKeepsSelection(t *testing.T) {
	var calls []struct {
		name  string
		delta int
	}
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{
		ReorderSession: func(name string, delta int) bool {
			calls = append(calls, struct {
				name  string
				delta int
			}{name: name, delta: delta})
			return true
		},
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "beta"}, {Name: "alpha"}, {Name: "gamma"}}
		},
	})

	updated, _ := model.Update(keyPress("j", tea.ModAlt))
	model = requireSidebarModel(t, updated)

	if len(calls) != 1 || calls[0].name != "alpha" || calls[0].delta != 1 {
		t.Fatalf("ReorderSession calls = %#v, want alpha/+1", calls)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after reorder = %#v ok=%v, want alpha", item, ok)
	}
}

func TestSidebarModelAltKAndAltArrowsReorderSelectedSession(t *testing.T) {
	tests := []struct {
		name      string
		key       tea.KeyPressMsg
		wantDelta int
	}{
		{name: "alt k", key: keyPress("k", tea.ModAlt), wantDelta: -1},
		{name: "alt down", key: tea.KeyPressMsg(tea.Key{Code: tea.KeyDown, Mod: tea.ModAlt}), wantDelta: 1},
		{name: "alt up", key: tea.KeyPressMsg(tea.Key{Code: tea.KeyUp, Mod: tea.ModAlt}), wantDelta: -1},
		{name: "alt shift j", key: tea.KeyPressMsg(tea.Key{Text: "J", Code: 'J', Mod: tea.ModAlt | tea.ModShift}), wantDelta: 1},
		{name: "alt shift k", key: tea.KeyPressMsg(tea.Key{Text: "K", Code: 'K', Mod: tea.ModAlt | tea.ModShift}), wantDelta: -1},
		{name: "alt shift up", key: tea.KeyPressMsg(tea.Key{Code: tea.KeyUp, Mod: tea.ModAlt | tea.ModShift}), wantDelta: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotDelta int
			model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{
				ReorderSession: func(name string, delta int) bool {
					gotName = name
					gotDelta = delta
					return true
				},
				ReloadSessions: func() []SessionItem { return []SessionItem{{Name: "alpha"}, {Name: "beta"}} },
			})
			model.cursor = 1

			updated, _ := model.Update(tt.key)
			model = requireSidebarModel(t, updated)

			if gotName != "beta" || gotDelta != tt.wantDelta {
				t.Fatalf("ReorderSession = %q/%d, want beta/%d", gotName, gotDelta, tt.wantDelta)
			}
			if item, ok := model.selectedSession(); !ok || item.Name != "beta" {
				t.Fatalf("selected after reorder = %#v ok=%v, want beta", item, ok)
			}
		})
	}
}

func TestSidebarModelLoadsAndPersistsShowNumericItems(t *testing.T) {
	var saved []bool
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}, {Name: "123"}}, Actions{
		SetShowNumericItems: func(show bool) bool {
			saved = append(saved, show)
			return true
		},
	}, SidebarOptions{ShowNumericItems: true})
	if !model.showNumeric {
		t.Fatal("showNumeric should load from options")
	}

	updated, _ := model.Update(keyPress("h", tea.ModAlt))
	model = requireSidebarModel(t, updated)

	if model.showNumeric {
		t.Fatal("showNumeric should toggle off")
	}
	if len(saved) != 1 || saved[0] {
		t.Fatalf("saved show numeric states = %#v, want [false]", saved)
	}
}

func TestSidebarModelAltHTogglesNumericSessionVisibility(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "lowercase", key: keyPress("h", tea.ModAlt)},
		{name: "uppercase text", key: keyPress("H", tea.ModAlt|tea.ModShift)},
		{name: "shifted keystroke", key: tea.KeyPressMsg(tea.Key{Text: "", Code: 'h', Mod: tea.ModAlt | tea.ModShift})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "123"}}, Actions{
				SetShowNumericItems: func(bool) bool { return true },
			})
			if strings.Contains(model.Render(), "123") {
				t.Fatalf("numeric session visible before toggle: %q", model.Render())
			}

			updated, _ := model.Update(tt.key)
			model = requireSidebarModel(t, updated)

			if !strings.Contains(model.Render(), "123") {
				t.Fatalf("numeric session hidden after toggle on: %q", model.Render())
			}
		})
	}
}

func TestSidebarModelKeepsShowNumericWhenPersistFails(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{
		SetShowNumericItems: func(bool) bool { return false },
	}, SidebarOptions{ShowNumericItems: true})

	updated, _ := model.Update(keyPress("h", tea.ModAlt))
	model = requireSidebarModel(t, updated)

	if !model.showNumeric {
		t.Fatal("showNumeric changed despite failed persistence")
	}
}

func TestSidebarModelRenderOmitsHeaderAndMovesFilterAboveHelp(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	view := model.Render()
	if strings.Contains(view, "sessions browse") {
		t.Fatalf("render includes removed header: %q", view)
	}

	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("a", 0))
	model = requireSidebarModel(t, updated)
	view = model.Render()
	filterIndex := strings.Index(view, "filter: a")
	helpIndex := strings.Index(view, "M-? keys")
	if filterIndex < 0 || helpIndex < 0 || filterIndex > helpIndex {
		t.Fatalf("filter should appear between list and help, view=%q", view)
	}
}

func TestSidebarModelEnterOnCurrentSessionDoesNothing(t *testing.T) {
	called := 0
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true}}, Actions{
		SwitchSession: func(string) bool {
			called++
			return true
		},
		ReloadSessions: func() []SessionItem {
			reloaded++
			return []SessionItem{{Name: "alpha", Current: true}}
		},
	})

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if called != 0 || reloaded != 0 {
		t.Fatalf("current session enter called SwitchSession %d times and ReloadSessions %d times, want neither", called, reloaded)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || !item.Current {
		t.Fatalf("selection changed after no-op enter: %#v ok=%v", item, ok)
	}
}

func TestSidebarModelChoosingProjectReturnsToBrowseMode(t *testing.T) {
	model := NewSidebarModel(nil, Actions{
		LoadProjects: func() []ProjectItem {
			return []ProjectItem{{Name: "tmux-stacked-panes", Path: "/projects/tmux-stacked-panes"}}
		},
		CreateProject:  func(ProjectItem) bool { return true },
		ReloadSessions: func() []SessionItem { return []SessionItem{{Name: "tmux-stacked-panes", Current: true}} },
	})

	updated, _ := model.Update(keyPress("n", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	for _, key := range []tea.KeyPressMsg{keyPress("s", 0), keyPress("t", 0), keyPress("a", 0), keyPress("c", 0)} {
		updated, _ = model.Update(key)
		model = requireSidebarModel(t, updated)
	}
	if model.mode != ModeProject || model.projectFilter != "stac" {
		t.Fatalf("setup failed: mode=%q projectFilter=%q", model.mode, model.projectFilter)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if model.mode != ModeBrowse || model.projectFilter != "" || model.projectCursor != 0 {
		t.Fatalf("project picker state not cleared after choose: %#v", model)
	}
	if len(model.items) != 1 || !model.items[0].Current {
		t.Fatalf("sessions not reloaded after choose: %#v", model.items)
	}
}

func TestSidebarModelFilterAcceptsSpace(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "space is printable filter input", keys: []tea.KeyPressMsg{keyPress("a", 0), keyPress(" ", 0), keyPress("b", 0)}, want: "a b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewSidebarModel([]SessionItem{{Name: "alpha beta"}}, Actions{})
			updated, _ := model.Update(keyPress("/", 0))
			model = requireSidebarModel(t, updated)
			for _, key := range tt.keys {
				updated, _ = model.Update(key)
				model = requireSidebarModel(t, updated)
			}
			if model.filter != tt.want {
				t.Fatalf("filter = %q, want %q", model.filter, tt.want)
			}
		})
	}
}

func TestSidebarModelRenderShowsAttentionMarkerInPrimaryMarkerColumn(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{})
	view := stripANSI(model.Render())
	if !strings.Contains(view, attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing attention marker in primary marker column in %q", view)
	}
	if strings.Contains(view, "alpha "+attentionMarkerSymbol) {
		t.Fatalf("render kept attention marker as a suffix in %q", view)
	}
}

func TestSidebarModelRenderUsesAttentionMarkerInsteadOfCurrentMarker(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true, Attention: true, Slot: 1}}, Actions{})
	view := stripANSI(model.Render())
	if !strings.Contains(view, attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing attention marker for current session in %q", view)
	}
	if strings.Contains(view, "* [1] alpha") {
		t.Fatalf("render kept current marker instead of bell in %q", view)
	}
}

func TestSidebarModelRenderDisplaysDoubleDigitSlots(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 10}, {Name: "beta", Slot: 11}}, Actions{})
	view := stripANSI(model.Render())
	for _, want := range []string{"[10] alpha", "[11] beta"} {
		if !strings.Contains(view, want) {
			t.Fatalf("render missing slot label %q in %q", want, view)
		}
	}
	if strings.Contains(view, "[0] alpha") {
		t.Fatalf("render used keyboard shortcut label instead of slot number in %q", view)
	}
}

func TestSidebarModelRenderKeepsAttentionMarkerWhiteWhenSessionTextIsStale(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "selected"}, {Name: "alpha", Attention: true, Slot: 1, Heat: string(heat.BucketStale)}}, Actions{})
	view := model.Render()
	if !strings.Contains(view, "\x1b[1;38;2;255;255;255m"+attentionMarkerSymbol) {
		t.Fatalf("render missing white attention marker in %q", view)
	}
	if !strings.Contains(view, "\x1b[38;2;75;85;99m[1] alpha") {
		t.Fatalf("render missing stale session text color in %q", view)
	}
}

func TestSidebarModelRenderAppliesSelectedStyleAcrossEntireRow(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1, Heat: string(heat.BucketStale)}}, Actions{})
	view := model.Render()
	if !strings.Contains(view, "48;2;6;95;70") {
		t.Fatalf("render missing selected row green background in %q", view)
	}
	if strings.Contains(view, "\x1b[38;2;75;85;99m[1] alpha") {
		t.Fatalf("selected row kept nested stale style that resets selected background in %q", view)
	}
}

func TestSidebarModelHelpToggleHidesExpandedFooterByDefault(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	view := model.Render()
	if !strings.Contains(view, "M-? keys") {
		t.Fatalf("collapsed footer missing help hint: %q", view)
	}
	if strings.Contains(view, "M-n project") || strings.Contains(view, "M-r rename") {
		t.Fatalf("collapsed footer includes expanded help: %q", view)
	}

	updated, _ := model.Update(keyPress("?", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	view = model.Render()
	for _, want := range []string{"↵ choose", "M-n project", "M-a adhoc", "M-H nums", "M-J/K reorder", "M-r rename", "M-x kill", "M-? hide"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded footer missing %q in %q", want, view)
		}
	}
}

func keyPress(text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0], Mod: mod})
}

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiRegexp.ReplaceAllString(value, "")
}

func requireSidebarModel(t *testing.T, model tea.Model) SidebarModel {
	t.Helper()
	m, ok := model.(SidebarModel)
	if !ok {
		t.Fatalf("Update returned %T, want SidebarModel", model)
	}
	return m
}
