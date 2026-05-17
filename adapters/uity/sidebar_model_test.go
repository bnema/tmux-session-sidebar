package uity

import (
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
	model = updated.(SidebarModel)
	if called != 0 {
		t.Fatalf("KillSession called before confirmation")
	}
	if model.mode != ModeConfirmKill || model.pendingKill != "alpha" {
		t.Fatalf("model confirmation state = mode %q pending %q", model.mode, model.pendingKill)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(SidebarModel)
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
	model = updated.(SidebarModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(SidebarModel)
	if called != 0 {
		t.Fatalf("KillSession called after cancellation")
	}
	if model.mode != ModeBrowse || model.pendingKill != "" || model.message != "" {
		t.Fatalf("model did not clear cancellation: %#v", model)
	}
}
