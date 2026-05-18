package uity

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(string) bool {
			called++
			return true
		},
	})
	updated, _ := model.Update(keyPress("x", tea.ModAlt))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	if called != 0 {
		t.Fatalf("KillSession called after cancellation")
	}
	if model.mode != ModeBrowse || model.pendingKill != "" || model.message != "" {
		t.Fatalf("model did not clear cancellation: %#v", model)
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

func TestSidebarModelFilterAcceptsSpace(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha beta"}}, Actions{})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	for _, key := range []tea.KeyPressMsg{keyPress("a", 0), keyPress(" ", 0), keyPress("b", 0)} {
		updated, _ = model.Update(key)
		model = requireSidebarModel(t, updated)
	}
	if model.filter != "a b" {
		t.Fatalf("filter = %q, want %q", model.filter, "a b")
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
	for _, want := range []string{"↵ choose", "M-n project", "M-a adhoc", "M-h nums", "M-J/K reorder", "M-r rename", "M-x kill", "M-? hide"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded footer missing %q in %q", want, view)
		}
	}
}

func keyPress(text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0], Mod: mod})
}

func requireSidebarModel(t *testing.T, model tea.Model) SidebarModel {
	t.Helper()
	m, ok := model.(SidebarModel)
	if !ok {
		t.Fatalf("Update returned %T, want SidebarModel", model)
	}
	return m
}
