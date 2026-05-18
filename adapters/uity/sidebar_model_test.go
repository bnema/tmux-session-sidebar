package uity

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	model = requireSidebarModel(t, updated)
	if called != 0 {
		t.Fatalf("KillSession called before confirmation")
	}
	if model.mode != ModeConfirmKill || model.pendingKill != "alpha" {
		t.Fatalf("model confirmation state = mode %q pending %q", model.mode, model.pendingKill)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
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

func TestSidebarModelKillConfirmationCanBeCancelled(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		KillSession: func(string) bool {
			called++
			return true
		},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = requireSidebarModel(t, updated)
	if called != 0 {
		t.Fatalf("KillSession called after cancellation")
	}
	if model.mode != ModeBrowse || model.pendingKill != "" || model.message != "" {
		t.Fatalf("model did not clear cancellation: %#v", model)
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
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	model = requireSidebarModel(t, updated)

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'?'}, Alt: true},
		{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true},
		{Type: tea.KeyF5},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
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

func TestSidebarModelHelpToggleHidesExpandedFooterByDefault(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	view := model.View()
	if !strings.Contains(view, "M-? keys") {
		t.Fatalf("collapsed footer missing help hint: %q", view)
	}
	if strings.Contains(view, "M-n project") || strings.Contains(view, "M-r rename") {
		t.Fatalf("collapsed footer includes expanded help: %q", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}, Alt: true})
	model = requireSidebarModel(t, updated)
	view = model.View()
	for _, want := range []string{"↵ choose", "M-n project", "M-a adhoc", "M-h nums", "M-r rename", "M-x kill", "M-? hide"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded footer missing %q in %q", want, view)
		}
	}
}

func requireSidebarModel(t *testing.T, model tea.Model) SidebarModel {
	t.Helper()
	m, ok := model.(SidebarModel)
	if !ok {
		t.Fatalf("Update returned %T, want SidebarModel", model)
	}
	return m
}
