package uity

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/core/heat"
)

func TestSidebarModelInitDoesNotSchedulePeriodicRefresh(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{})
	if cmd := model.Init(); cmd != nil {
		t.Fatal("Init() scheduled a periodic refresh command")
	}
}

func TestSidebarModelViewEnablesMouseWheelEvents(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})

	view := model.View()

	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("View().MouseMode = %v, want %v", view.MouseMode, tea.MouseModeCellMotion)
	}
}

func TestSidebarModelMouseWheelNavigatesSessions(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{})

	updated, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" {
		t.Fatalf("selection after wheel down = %#v ok=%v, want beta", item, ok)
	}

	updated, _ = model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	model = requireSidebarModel(t, updated)
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selection after wheel up = %#v ok=%v, want alpha", item, ok)
	}
}

func TestSidebarModelMouseWheelNavigatesFilteredSessions(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "bravo"}}, Actions{})
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)

	updated, _ = model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	model = requireSidebarModel(t, updated)

	if item, ok := model.selectedSession(); !ok || item.Name != "bravo" {
		t.Fatalf("selection after filtered wheel down = %#v ok=%v, want bravo", item, ok)
	}
}

func TestSidebarModelSelfUpdateShowsSpinner(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SelfUpdate: func() tea.Cmd {
		return func() tea.Msg { return SelfUpdateFinishedMsg{} }
	}})

	updated, cmd := model.Update(keyPress("u", 0))
	model = requireSidebarModel(t, updated)
	if !model.updateInProgress {
		t.Fatal("u shortcut did not mark update in progress")
	}
	if cmd == nil {
		t.Fatal("u shortcut did not schedule spinner tick")
	}
	if !strings.Contains(stripANSI(model.Render()), "Updating runtime") {
		t.Fatalf("render missing update spinner message: %q", stripANSI(model.Render()))
	}
}

func TestSidebarModelSelfUpdateFinishedStopsSpinnerAndReportsFailure(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SelfUpdate: func() tea.Cmd {
		return func() tea.Msg { return SelfUpdateFinishedMsg{Err: errors.New("download failed")} }
	}})

	updated, cmd := model.Update(keyPress("u", 0))
	model = requireSidebarModel(t, updated)
	if cmd == nil || !model.updateInProgress {
		t.Fatal("self-update did not start spinner before completion")
	}

	updated, cmd = model.Update(SelfUpdateFinishedMsg{Err: errors.New("download failed")})
	model = requireSidebarModel(t, updated)
	if model.updateInProgress {
		t.Fatal("self-update failure did not stop spinner")
	}
	if cmd != nil {
		t.Fatal("self-update failure scheduled unexpected command")
	}
	if !strings.Contains(model.message, "Update failed: download failed") {
		t.Fatalf("message = %q, want failure message", model.message)
	}
}

func TestSidebarModelSelfUpdateIgnoresRepeatedLaunchWhileInProgress(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SelfUpdate: func() tea.Cmd {
		called++
		return func() tea.Msg { return SelfUpdateFinishedMsg{} }
	}})

	updated, cmd := model.Update(keyPress("u", 0))
	model = requireSidebarModel(t, updated)
	if cmd == nil || !model.updateInProgress || called != 1 {
		t.Fatalf("first update state: cmd nil=%v inProgress=%v called=%d", cmd == nil, model.updateInProgress, called)
	}

	updated, cmd = model.Update(keyPress("u", 0))
	model = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("repeated u scheduled another command")
	}
	if called != 1 {
		t.Fatalf("repeated u called SelfUpdate %d times, want 1", called)
	}
}

func TestSidebarModelSelfUpdateDoesNotSpinWhenLaunchFails(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SelfUpdate: func() tea.Cmd { return nil }})

	updated, cmd := model.Update(keyPress("u", 0))
	model = requireSidebarModel(t, updated)
	if model.updateInProgress {
		t.Fatal("failed self-update launch marked update in progress")
	}
	if cmd != nil {
		t.Fatal("failed self-update launch scheduled spinner tick")
	}
}

func TestSidebarModelQuestionMarkTogglesHelpOnlyOutsideSearch(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})

	updated, _ := model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	if !model.showHelp {
		t.Fatal("? in browse mode did not show help")
	}

	updated, _ = model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	if model.showHelp {
		t.Fatal("second ? in browse mode did not hide help")
	}

	updated, _ = model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	if model.showHelp {
		t.Fatal("? while searching should not toggle help")
	}
	if model.filter != "?" {
		t.Fatalf("search filter = %q, want ?", model.filter)
	}
}

func TestSidebarModelSingleKeyBrowseShortcuts(t *testing.T) {
	t.Run("n opens project mode", func(t *testing.T) {
		loaded := 0
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{LoadProjects: func() []ProjectItem {
			loaded++
			return []ProjectItem{{Name: "proj", Path: "/tmp/proj"}}
		}})
		updated, _ := model.Update(keyPress("n", 0))
		model = requireSidebarModel(t, updated)
		if loaded != 1 || model.mode != ModeProject {
			t.Fatalf("n shortcut loaded=%d mode=%s, want loaded once and project mode", loaded, model.mode)
		}
	})

	t.Run("g creates git project", func(t *testing.T) {
		called := 0
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{CreateGitProject: func() bool { called++; return false }})
		updated, _ := model.Update(keyPress("g", 0))
		_ = requireSidebarModel(t, updated)
		if called != 1 {
			t.Fatalf("g shortcut called CreateGitProject %d times, want 1", called)
		}
	})

	t.Run("a creates adhoc", func(t *testing.T) {
		called := 0
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{CreateAdhoc: func() bool { called++; return false }})
		updated, _ := model.Update(keyPress("a", 0))
		_ = requireSidebarModel(t, updated)
		if called != 1 {
			t.Fatalf("a shortcut called CreateAdhoc %d times, want 1", called)
		}
	})

	t.Run("r renames selected session", func(t *testing.T) {
		var renamed string
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{RenameSession: func(name string) bool { renamed = name; return false }})
		updated, _ := model.Update(keyPress("r", 0))
		_ = requireSidebarModel(t, updated)
		if renamed != "alpha" {
			t.Fatalf("r shortcut renamed %q, want alpha", renamed)
		}
	})

	t.Run("x starts kill confirmation", func(t *testing.T) {
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{KillSession: func(string) bool { return true }})
		updated, _ := model.Update(keyPress("x", 0))
		model = requireSidebarModel(t, updated)
		if model.mode != ModeConfirmKill || model.pendingKill != "alpha" {
			t.Fatalf("x shortcut mode=%s pending=%q, want confirm kill alpha", model.mode, model.pendingKill)
		}
	})

	t.Run("h toggles numeric sessions", func(t *testing.T) {
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SetShowNumericItems: func(show bool) bool { return show }})
		updated, _ := model.Update(keyPress("h", 0))
		model = requireSidebarModel(t, updated)
		if !model.showNumeric {
			t.Fatal("h shortcut did not toggle numeric sessions on")
		}
	})

	t.Run("u starts self update", func(t *testing.T) {
		called := 0
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{SelfUpdate: func() tea.Cmd {
			called++
			return func() tea.Msg { return SelfUpdateFinishedMsg{} }
		}})
		updated, _ := model.Update(keyPress("u", 0))
		model = requireSidebarModel(t, updated)
		if called != 1 {
			t.Fatalf("u shortcut called SelfUpdate %d times, want 1", called)
		}
		if !model.updateInProgress || !strings.Contains(stripANSI(model.message), "Updating runtime") {
			t.Fatalf("update state/message = %v/%q, want spinner update message", model.updateInProgress, stripANSI(model.message))
		}
	})

	t.Run("shift j and k reorder selected session", func(t *testing.T) {
		var deltas []int
		model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{ReorderSession: func(name string, delta int) bool {
			if name != "alpha" {
				t.Fatalf("reordered session = %q, want alpha", name)
			}
			deltas = append(deltas, delta)
			return false
		}})
		updated, _ := model.Update(keyPress("J", tea.ModShift))
		model = requireSidebarModel(t, updated)
		updated, _ = model.Update(keyPress("K", tea.ModShift))
		_ = requireSidebarModel(t, updated)
		if fmt.Sprint(deltas) != "[1 -1]" {
			t.Fatalf("J/K reorder deltas = %v, want [1 -1]", deltas)
		}
	})
}

func TestSidebarModelSingleKeyShortcutsRemainSearchInput(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		LoadProjects:        func() []ProjectItem { t.Fatal("n should not open projects while searching"); return nil },
		CreateGitProject:    func() bool { t.Fatal("g should not create git project while searching"); return false },
		CreateAdhoc:         func() bool { t.Fatal("a should not create adhoc while searching"); return false },
		RenameSession:       func(string) bool { t.Fatal("r should not rename while searching"); return false },
		KillSession:         func(string) bool { t.Fatal("x should not kill while searching"); return false },
		SetShowNumericItems: func(bool) bool { t.Fatal("h should not toggle numeric while searching"); return false },
		SelfUpdate:          func() tea.Cmd { t.Fatal("u should not update while searching"); return nil },
		ReorderSession:      func(string, int) bool { t.Fatal("J/K should not reorder while searching"); return false },
	})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	for _, key := range []tea.KeyPressMsg{keyPress("n", 0), keyPress("g", 0), keyPress("a", 0), keyPress("r", 0), keyPress("x", 0), keyPress("h", 0), keyPress("u", 0), keyPress("J", tea.ModShift), keyPress("K", tea.ModShift)} {
		updated, _ = model.Update(key)
		model = requireSidebarModel(t, updated)
	}
	if model.filter != "ngarxhuJK" {
		t.Fatalf("search filter = %q, want ngarxhuJK", model.filter)
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

func TestSidebarModelF5SelectsPreviousCurrentSessionAfterExternalSwitch(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true}, {Name: "beta"}}, Actions{
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha"}, {Name: "beta", Current: true}}
		},
	})

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	model = requireSidebarModel(t, updated)

	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after external switch reload = %#v ok=%v, want previous alpha", item, ok)
	}
}

func TestSidebarModelSwitchSelectedStartsAttentionAnimationAndSelectsPreviousCurrentAfterReload(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Current: true, Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool { return name == "beta" },
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha", Attention: true, Slot: 1}, {Name: "beta", Current: true, Slot: 2}}
		},
	}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	model.cursor = 1

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if cmd == nil {
		t.Fatal("switch reload did not schedule attention animation tick")
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after switch reload = %#v ok=%v, want previous alpha", item, ok)
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

	updated, _ := model.Update(keyPress("x", 0))
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
			updated, _ := model.Update(keyPress("x", 0))
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
			updated, _ := model.Update(keyPress("x", 0))
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
	updated, _ := model.Update(keyPress("x", 0))
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
	updated, _ := model.Update(keyPress("x", 0))
	model = requireSidebarModel(t, updated)

	for _, key := range []tea.KeyPressMsg{
		keyPress("?", 0),
		keyPress("h", 0),
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

func TestSidebarModelNumberKeySwitchesDisplayedSlot(t *testing.T) {
	called := 0
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1, Current: true}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool {
			called++
			if name != "beta" {
				t.Fatalf("SwitchSession called with %q, want beta", name)
			}
			return true
		},
		ReloadSessions: func() []SessionItem {
			reloaded++
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Slot: 2, Current: true}}
		},
	})

	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("SwitchSession called %d times, want 1", called)
	}
	if reloaded != 1 {
		t.Fatalf("ReloadSessions called %d times, want 1", reloaded)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after slot switch = %#v ok=%v, want previous alpha", item, ok)
	}
}

func TestSidebarModelZeroKeySwitchesDisplayedSlotTen(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1, Current: true}, {Name: "kappa", Slot: 10}}, Actions{
		SwitchSession: func(name string) bool {
			called++
			if name != "kappa" {
				t.Fatalf("SwitchSession called with %q, want kappa", name)
			}
			return true
		},
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha", Slot: 1}, {Name: "kappa", Slot: 10, Current: true}}
		},
	})

	updated, _ := model.Update(keyPress("0", 0))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("SwitchSession called %d times, want 1", called)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selected after slot 10 switch = %#v ok=%v, want previous alpha", item, ok)
	}
}

func TestSidebarModelNumberKeyDoesNothingWhenSlotMissing(t *testing.T) {
	called := 0
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession:  func(string) bool { called++; return true },
		ReloadSessions: func() []SessionItem { reloaded++; return nil },
	})
	model.cursor = 1

	updated, _ := model.Update(keyPress("9", 0))
	model = requireSidebarModel(t, updated)

	if called != 0 || reloaded != 0 {
		t.Fatalf("missing slot called SwitchSession %d times and ReloadSessions %d times, want neither", called, reloaded)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "beta" {
		t.Fatalf("selection changed after missing slot: %#v ok=%v", item, ok)
	}
}

func TestSidebarModelNumberKeyDoesNotReloadWhenSwitchFails(t *testing.T) {
	called := 0
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1}, {Name: "beta", Slot: 2}}, Actions{
		SwitchSession: func(name string) bool {
			called++
			if name != "beta" {
				t.Fatalf("SwitchSession called with %q, want beta", name)
			}
			return false
		},
		ReloadSessions: func() []SessionItem { reloaded++; return nil },
	})

	updated, _ := model.Update(keyPress("2", 0))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("SwitchSession called %d times, want 1", called)
	}
	if reloaded != 0 {
		t.Fatalf("ReloadSessions called %d times after failed switch, want 0", reloaded)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selection changed after failed switch: %#v ok=%v", item, ok)
	}
}

func TestSidebarModelNumberKeyFiltersInSearchMode(t *testing.T) {
	var switched string
	model := NewSidebarModel([]SessionItem{{Name: "alpha1", Slot: 1}}, Actions{
		SwitchSession: func(name string) bool {
			switched = name
			return true
		},
	})
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)

	updated, _ = model.Update(keyPress("1", 0))
	model = requireSidebarModel(t, updated)

	if switched != "" {
		t.Fatalf("SwitchSession called in search mode with %q", switched)
	}
	if model.filter != "1" {
		t.Fatalf("search filter = %q, want 1", model.filter)
	}
}

func TestSidebarModelShiftJReordersSelectedSessionDownAndKeepsSelection(t *testing.T) {
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

	updated, _ := model.Update(keyPress("J", tea.ModShift))
	model = requireSidebarModel(t, updated)

	if len(calls) != 1 || calls[0].name != "alpha" || calls[0].delta != 1 {
		t.Fatalf("ReorderSession calls = %#v, want alpha/+1", calls)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" {
		t.Fatalf("selected after reorder = %#v ok=%v, want alpha", item, ok)
	}
}

func TestSidebarModelShiftKReordersSelectedSession(t *testing.T) {
	tests := []struct {
		name      string
		key       tea.KeyPressMsg
		wantDelta int
	}{
		{name: "shift j", key: keyPress("J", tea.ModShift), wantDelta: 1},
		{name: "shift k", key: keyPress("K", tea.ModShift), wantDelta: -1},
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

	updated, _ := model.Update(keyPress("h", 0))
	model = requireSidebarModel(t, updated)

	if model.showNumeric {
		t.Fatal("showNumeric should toggle off")
	}
	if len(saved) != 1 || saved[0] {
		t.Fatalf("saved show numeric states = %#v, want [false]", saved)
	}
}

func TestSidebarModelHTogglesNumericSessionVisibility(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "123"}}, Actions{
		SetShowNumericItems: func(bool) bool { return true },
	})
	if strings.Contains(model.Render(), "123") {
		t.Fatalf("numeric session visible before toggle: %q", model.Render())
	}

	updated, _ := model.Update(keyPress("h", 0))
	model = requireSidebarModel(t, updated)

	if !strings.Contains(model.Render(), "123") {
		t.Fatalf("numeric session hidden after toggle on: %q", model.Render())
	}
}

func TestSidebarModelKeepsShowNumericWhenPersistFails(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{
		SetShowNumericItems: func(bool) bool { return false },
	}, SidebarOptions{ShowNumericItems: true})

	updated, _ := model.Update(keyPress("h", 0))
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
	helpIndex := strings.Index(view, "? keys")
	if filterIndex < 0 || helpIndex < 0 || filterIndex > helpIndex {
		t.Fatalf("filter should appear between list and help, view=%q", view)
	}
}

func TestSidebarModelRenderShowsMetadataSublineBelowSessionRow(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1, Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Branch: "main", Modified: 2}}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	model = requireSidebarModel(t, updated)

	view := stripANSI(model.Render())
	if !strings.Contains(view, " [1] alpha \n        main  2") {
		t.Fatalf("render should include metadata subline below session row: %q", view)
	}
}

func TestSidebarModelRenderBrightensSelectedRecentMetadataSubline(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Slot: 1, Heat: string(heat.BucketHot), Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Modified: 2}}}, Actions{})
	view := model.Render()

	if !strings.Contains(view, "38;2;253;224;71") {
		t.Fatalf("selected recent metadata should use active part colors: %q", view)
	}
}

func TestSidebarModelRenderColorsSelectedRecentGitMetadataParts(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Heat: string(heat.BucketHot), Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}}}, Actions{})
	view := model.Render()

	for _, want := range []string{"38;2;125;211;252", "38;2;134;239;172", "38;2;248;113;113", "38;2;147;197;253", "38;2;253;224;71"} {
		if !strings.Contains(view, want) {
			t.Fatalf("recent metadata should include color %s, view=%q", want, view)
		}
	}
}

func TestSidebarModelRenderColorsUnselectedRecentGitMetadataParts(t *testing.T) {
	model := NewSidebarModel([]SessionItem{
		{Name: "alpha"},
		{Name: "beta", Heat: string(heat.BucketHot), Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}},
	}, Actions{})
	view := model.Render()

	for _, want := range []string{"38;2;56;189;248", "38;2;74;222;128", "38;2;248;113;113", "38;2;96;165;250", "38;2;234;179;8"} {
		if !strings.Contains(view, want) {
			t.Fatalf("unselected recent metadata should include color %s, view=%q", want, view)
		}
	}
}

func TestSidebarModelRenderDesaturatesStaleGitMetadata(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Heat: string(heat.BucketStale), Metadata: SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}}}, Actions{})
	view := model.Render()

	for _, forbidden := range []string{"38;2;125;211;252", "38;2;56;189;248", "38;2;134;239;172", "38;2;74;222;128", "38;2;248;113;113", "38;2;147;197;253", "38;2;96;165;250", "38;2;253;224;71", "38;2;234;179;8"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("stale metadata should not include part color %s, view=%q", forbidden, view)
		}
	}
	if !strings.Contains(stripANSI(view), " 12 -2  3  8") {
		t.Fatalf("stale metadata should still render desaturated content, view=%q", view)
	}
	if !strings.Contains(view, "38;2;75;85;99") {
		t.Fatalf("stale metadata should use inactive dark gray, view=%q", view)
	}
}

func TestSidebarModelRenderCompactsMetadataSublineToWindowWidth(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{
		Name: "alpha",
		Metadata: SessionMetadataSubline{
			Kind:     MetadataKindGit,
			Branch:   "feature/add-session-metadata-subline",
			Ahead:    2,
			Modified: 3,
		},
	}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	model = requireSidebarModel(t, updated)

	lines := strings.Split(stripANSI(model.Render()), "\n")
	if len(lines) < 3 || !strings.Contains(lines[2], " 2  3") {
		t.Fatalf("metadata subline should be width-aware, lines=%q", lines)
	}
	if width := metadataDisplayWidth(strings.TrimSpace(lines[2])); width > 24 {
		t.Fatalf("metadata subline width = %d, want <= 24: %q", width, lines[2])
	}
}

func TestSidebarModelRenderOmitsMetadataSublineWhenUnavailable(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})

	view := stripANSI(model.Render())
	if strings.Contains(view, "\n    git") || strings.Contains(view, "\n    ") {
		t.Fatalf("render should not include placeholder metadata before async data is available: %q", view)
	}
}

func TestSidebarModelRenderShowsVersionInCollapsedHelp(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Version: "0.10.2"})
	view := stripANSI(model.Render())
	if !strings.Contains(view, " v0.10.2  ? keys") {
		t.Fatalf("render should include versioned help hint: %q", view)
	}
}

func TestSidebarModelRenderShowsGreenUpdateIndicatorForDevBuilds(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Version: "dev"})
	view := model.Render()
	if !strings.Contains(stripANSI(view), " dev"+updateAvailableSymbol+"  ? keys") {
		t.Fatalf("render should include update indicator attached to dev version: %q", view)
	}
	if !strings.Contains(view, "38;2;34;197;94") {
		t.Fatalf("render should color update indicator green: %q", view)
	}
	if !strings.Contains(view, "48;2;51;65;85") {
		t.Fatalf("render should keep update indicator in the version badge background: %q", view)
	}
}

func TestSidebarModelInitSkipsInvalidReleaseVersions(t *testing.T) {
	called := false
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{
		Version: "source",
		CheckUpdateAvailable: func(string) (bool, error) {
			called = true
			return true, nil
		},
	})
	if cmd := model.Init(); cmd != nil {
		t.Fatal("Init returned update check command for invalid version")
	}
	if called {
		t.Fatal("update checker was called for invalid version")
	}
}

func TestSidebarModelInitChecksForUpdatesAsynchronously(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{
		Version: "0.10.2",
		CheckUpdateAvailable: func(current string) (bool, error) {
			if current != "0.10.2" {
				t.Fatalf("current version = %q, want 0.10.2", current)
			}
			close(started)
			<-release
			return true, nil
		},
	})
	if strings.Contains(stripANSI(model.Render()), updateAvailableSymbol) {
		t.Fatalf("render should not include update indicator before async check completes: %q", model.Render())
	}

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init returned nil command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("Init command = %#v, want batch with update check", cmd())
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- batch[0]() }()
	<-started
	if strings.Contains(stripANSI(model.Render()), updateAvailableSymbol) {
		t.Fatalf("render should not include update indicator while async check is blocked: %q", model.Render())
	}
	close(release)
	updated, _ := model.Update(<-result)
	model = requireSidebarModel(t, updated)
	if !strings.Contains(stripANSI(model.Render()), " v0.10.2"+updateAvailableSymbol+"  ? keys") {
		t.Fatalf("render should include update indicator attached to version after async check completes: %q", model.Render())
	}
}

func TestSidebarModelRenderStylesVersionBadge(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Version: "dev"})
	view := model.Render()
	if !strings.Contains(view, "48;2;51;65;85") {
		t.Fatalf("render should give version badge a distinct background: %q", view)
	}
	if !strings.Contains(stripANSI(view), " dev"+updateAvailableSymbol+"  ? keys") {
		t.Fatalf("render should pad the version badge text and show dev update indicator: %q", view)
	}
	lastLine := lastRenderedLine(view)
	if strings.HasPrefix(lastLine, " ") {
		t.Fatalf("statusbar should not have unstyled leading padding before badge: %q", view)
	}
}

func TestSidebarModelAnchorsCollapsedStatusBarToWindowBottom(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Version: "dev"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 7})
	model = requireSidebarModel(t, updated)

	lines := strings.Split(stripANSI(model.Render()), "\n")
	if len(lines) != 7 {
		t.Fatalf("rendered height = %d, want 7; lines=%q", len(lines), lines)
	}
	if !strings.Contains(lines[len(lines)-1], " dev"+updateAvailableSymbol+"  ? keys") {
		t.Fatalf("statusbar should be on the last line: %q", lines)
	}
}

func TestSidebarModelRecalculatesStatusBarPositionAfterResize(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{Version: "dev"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 7})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	model = requireSidebarModel(t, updated)

	lines := strings.Split(stripANSI(model.Render()), "\n")
	if len(lines) != 5 {
		t.Fatalf("rendered height after resize = %d, want 5; lines=%q", len(lines), lines)
	}
	if !strings.Contains(lines[len(lines)-1], " dev"+updateAvailableSymbol+"  ? keys") {
		t.Fatalf("statusbar should remain on the last line after resize: %q", lines)
	}
}

func TestSidebarModelEnterSwitchesAndSelectsPreviousCurrentSession(t *testing.T) {
	called := 0
	reloaded := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true}, {Name: "beta"}}, Actions{
		SwitchSession: func(name string) bool {
			called++
			if name != "beta" {
				t.Fatalf("SwitchSession called with %q, want beta", name)
			}
			return true
		},
		ReloadSessions: func() []SessionItem {
			reloaded++
			return []SessionItem{{Name: "alpha"}, {Name: "beta", Current: true}}
		},
	})

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = requireSidebarModel(t, updated)
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("SwitchSession called %d times, want 1", called)
	}
	if reloaded != 1 {
		t.Fatalf("ReloadSessions called %d times after switch, want 1", reloaded)
	}
	if cmd != nil {
		t.Fatal("Update(Enter) returned an unexpected follow-up command")
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Current {
		t.Fatalf("selection after switch = %#v ok=%v, want previous alpha", item, ok)
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

func TestSidebarModelCreateActionsSelectNewCurrentSession(t *testing.T) {
	tests := []struct {
		name   string
		key    tea.KeyPressMsg
		action func(*Actions)
	}{
		{name: "git project", key: keyPress("g", 0), action: func(actions *Actions) { actions.CreateGitProject = func() bool { return true } }},
		{name: "adhoc", key: keyPress("a", 0), action: func(actions *Actions) { actions.CreateAdhoc = func() bool { return true } }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := Actions{
				ReloadSessions: func() []SessionItem { return []SessionItem{{Name: "alpha"}, {Name: "created", Current: true}} },
			}
			tt.action(&actions)
			model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true}, {Name: "created"}}, actions)

			updated, _ := model.Update(tt.key)
			model = requireSidebarModel(t, updated)

			if item, ok := model.selectedSession(); !ok || item.Name != "created" || !item.Current {
				t.Fatalf("selected after create = %#v ok=%v, want created current", item, ok)
			}
		})
	}
}

func TestSidebarModelChoosingProjectReturnsToBrowseMode(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Current: true}}, Actions{
		LoadProjects: func() []ProjectItem {
			return []ProjectItem{{Name: "tmux-stacked-panes", Path: "/projects/tmux-stacked-panes"}}
		},
		CreateProject: func(ProjectItem) bool { return true },
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha"}, {Name: "tmux-stacked-panes", Current: true}}
		},
	})

	updated, _ := model.Update(keyPress("n", 0))
	model = requireSidebarModel(t, updated)
	for _, key := range []tea.KeyPressMsg{keyPress("s", 0), keyPress("t", 0), keyPress("a", 0), keyPress("c", 0)} {
		updated, _ = model.Update(key)
		model = requireSidebarModel(t, updated)
	}
	if model.mode != ModeProject || model.menu.Filter != "stac" {
		t.Fatalf("setup failed: mode=%q menuFilter=%q", model.mode, model.menu.Filter)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	if model.mode != ModeBrowse || model.menu.Filter != "" || model.menu.Cursor != 0 {
		t.Fatalf("project picker state not cleared after choose: %#v", model)
	}
	if len(model.items) != 2 || !model.items[1].Current {
		t.Fatalf("sessions not reloaded after choose: %#v", model.items)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "tmux-stacked-panes" || !item.Current {
		t.Fatalf("selected after project create = %#v ok=%v, want new current project", item, ok)
	}
}

func TestSidebarModelSpaceOpensPinColorPickerForUnpinnedSession(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{
		PinSessionWithColor: func(name string, color string) bool {
			called++
			return true
		},
	})

	updated, _ := model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	if called != 0 {
		t.Fatalf("PinSessionWithColor called before confirmation")
	}
	if model.mode != ModePinColor || model.pinColorSession != "alpha" {
		t.Fatalf("model pin color state = mode %q session %q, want pin-color alpha", model.mode, model.pinColorSession)
	}
	view := stripANSI(model.Render())
	if !strings.Contains(view, "pin color") || !strings.Contains(view, "↵/sp ok esc") {
		t.Fatalf("render missing compact pin color picker in %q", view)
	}
}

func TestSidebarModelPinColorPickerOverlaysNarrowSidebar(t *testing.T) {
	items := make([]SessionItem, 0, 30)
	for i := range 30 {
		items = append(items, SessionItem{Name: fmt.Sprintf("session-%02d", i), Slot: i + 1})
	}
	model := NewSidebarModel(items, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	view := stripANSI(model.Render())
	lines := strings.Split(view, "\n")
	if len(lines) != 10 {
		t.Fatalf("rendered height = %d, want 10; view=%q", len(lines), view)
	}
	if !strings.Contains(view, "pin color") {
		t.Fatalf("render missing overlaid pin color picker in %q", view)
	}
	for _, line := range lines {
		if width := len([]rune(line)); width > 20 {
			t.Fatalf("rendered line width = %d, want <= 20; line=%q view=%q", width, line, view)
		}
	}
}

func TestSidebarModelPinColorPickerUsesLargeSwatches(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	view := stripANSI(model.Render())
	if !strings.Contains(view, "████") {
		t.Fatalf("render missing wide color swatches in %q", view)
	}
}

func TestSidebarModelPinColorPickerConfirmsSelectedColor(t *testing.T) {
	var gotName, gotColor string
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{
		PinSessionWithColor: func(name string, color string) bool {
			gotName = name
			gotColor = color
			return true
		},
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha", Pinned: true, PinColor: pinColorPalette[1]}, {Name: "beta"}}
		},
	})

	updated, _ := model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	if gotName != "alpha" || gotColor != pinColorPalette[1] {
		t.Fatalf("PinSessionWithColor = (%q, %q), want (alpha, %s)", gotName, gotColor, pinColorPalette[1])
	}
	if model.mode != ModeBrowse || model.pinColorSession != "" {
		t.Fatalf("picker did not close after confirmation: %#v", model)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || !item.Pinned || item.PinColor != pinColorPalette[1] {
		t.Fatalf("selected after pin color confirm = %#v ok=%v, want pinned alpha blue", item, ok)
	}
}

func TestSidebarModelPinColorPickerStaysOpenWhenPinActionFails(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		PinSessionWithColor: func(string, string) bool {
			called++
			return false
		},
		ReloadSessions: func() []SessionItem {
			t.Fatal("ReloadSessions should not run when pin action fails")
			return nil
		},
	})

	updated, _ := model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("PinSessionWithColor called %d times, want 1", called)
	}
	if model.mode != ModePinColor || model.pinColorSession != "alpha" {
		t.Fatalf("picker state after failed pin = mode %q session %q, want still open for alpha", model.mode, model.pinColorSession)
	}
}

func TestSidebarModelPinColorPickerNavigatesWithVimKeysAndCancels(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{
		PinSessionWithColor: func(string, string) bool { called++; return true },
	})
	updated, _ := model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("l", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("j", 0))
	model = requireSidebarModel(t, updated)
	if model.pinColorPicker.Cursor != 5 {
		t.Fatalf("pinColorCursor after l,j = %d, want 5", model.pinColorPicker.Cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = requireSidebarModel(t, updated)
	if called != 0 || model.mode != ModeBrowse || model.pinColorSession != "" {
		t.Fatalf("cancel state called=%d model=%#v", called, model)
	}
}

func TestSidebarModelSpaceUnpinsPinnedSessionDirectly(t *testing.T) {
	called := 0
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Pinned: true}, {Name: "beta"}}, Actions{
		TogglePinnedSession: func(name string) bool {
			called++
			if name != "alpha" {
				t.Fatalf("TogglePinnedSession called with %q, want alpha", name)
			}
			return true
		},
		ReloadSessions: func() []SessionItem { return []SessionItem{{Name: "alpha"}, {Name: "beta"}} },
	})

	updated, _ := model.Update(keyPress(" ", 0))
	model = requireSidebarModel(t, updated)

	if called != 1 {
		t.Fatalf("TogglePinnedSession called %d times, want 1", called)
	}
	if model.mode != ModeBrowse {
		t.Fatalf("mode after unpin = %q, want browse", model.mode)
	}
	if item, ok := model.selectedSession(); !ok || item.Name != "alpha" || item.Pinned {
		t.Fatalf("selected after unpin = %#v ok=%v, want unpinned alpha", item, ok)
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

func TestSidebarModelRenderShowsPinnedMarkerInPrimaryMarkerColumn(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "selected"}, {Name: "alpha", Pinned: true, Slot: 1}}, Actions{})
	model.cursor = 0
	view := stripANSI(model.Render())
	if !strings.Contains(view, pinnedMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing pinned marker in primary marker column in %q", view)
	}
}

func TestSidebarModelRenderShowsPinColorMarkerForSelectedPinnedSession(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha", Pinned: true, PinColor: "#38bdf8", Slot: 1}}, Actions{})
	view := model.Render()
	if !strings.Contains(view, "38;2;56;189;248") {
		t.Fatalf("render missing selected pinned session color marker in %q", view)
	}
	if strings.Count(view, "48;2;6;95;70") < 2 {
		t.Fatalf("render should keep selected background on marker and session text in %q", view)
	}
}

func TestSidebarModelRenderShowsCurrentMarkerForCurrentPinnedSession(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "selected"}, {Name: "alpha", Current: true, Pinned: true, Slot: 1}}, Actions{})
	model.cursor = 0
	view := model.Render()
	stripped := stripANSI(view)
	if !strings.Contains(stripped, currentMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing current marker for current pinned session in %q", stripped)
	}
	if strings.Contains(stripped, pinnedMarkerSymbol+" [1] alpha") {
		t.Fatalf("render kept pinned marker for current pinned session in %q", stripped)
	}
	if !strings.Contains(view, "38;2;250;204;21") {
		t.Fatalf("render missing pinned yellow color for current pinned session in %q", view)
	}
}

func TestSidebarModelRenderShowsAttentionMarkerInPrimaryMarkerColumn(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	view := stripANSI(model.Render())
	if !strings.Contains(view, attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing attention marker in primary marker column in %q", view)
	}
	if strings.Contains(view, "alpha "+attentionMarkerSymbol) {
		t.Fatalf("render kept attention marker as a suffix in %q", view)
	}
}

func TestSidebarModelRenderUsesAttentionMarkerInsteadOfCurrentMarker(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Current: true, Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	view := stripANSI(model.Render())
	if !strings.Contains(view, attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("render missing attention marker for current session in %q", view)
	}
	if strings.Contains(view, currentMarkerSymbol+" [1] alpha") {
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
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "selected"}, {Name: "alpha", Attention: true, Slot: 1, Heat: string(heat.BucketStale)}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	view := model.Render()
	if !strings.Contains(view, "\x1b[1;38;2;255;255;255m"+attentionMarkerSymbol) {
		t.Fatalf("render missing white attention marker in %q", view)
	}
	if !strings.Contains(view, "\x1b[38;2;75;85;99m[1] alpha") {
		t.Fatalf("render missing stale session text color in %q", view)
	}
}

func TestSidebarModelInitSchedulesAttentionAnimationWhenAttentionIsVisible(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	if cmd := model.Init(); cmd == nil {
		t.Fatal("Init() did not schedule attention animation tick")
	}
}

func TestSidebarModelAttentionAnimationTickAdvancesFrameWhileAttentionExists(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})

	updated, cmd := model.Update(attentionAnimationTickMsg{Generation: untrackedAttentionAnimationTick})
	model = requireSidebarModel(t, updated)

	if model.attentionAnimationFrame != 1 {
		t.Fatalf("attentionAnimationFrame = %d, want 1", model.attentionAnimationFrame)
	}
	if cmd == nil {
		t.Fatal("Update(attentionAnimationTickMsg) did not schedule the next tick")
	}
}

func TestSidebarModelAttentionAnimationTickStopsWithoutAttention(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})

	updated, cmd := model.Update(attentionAnimationTickMsg{Generation: untrackedAttentionAnimationTick})
	model = requireSidebarModel(t, updated)

	if model.attentionAnimationFrame != 0 {
		t.Fatalf("attentionAnimationFrame = %d, want 0", model.attentionAnimationFrame)
	}
	if cmd != nil {
		t.Fatal("Update(attentionAnimationTickMsg) scheduled a tick without attention")
	}
}

func TestSidebarModelAttentionAnimationDoesNotDuplicatePendingTickAcrossReloads(t *testing.T) {
	reloads := [][]SessionItem{
		{{Name: "alpha", Slot: 1}},
		{{Name: "alpha", Attention: true, Slot: 1}},
	}
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{
		ReloadSessions: func() []SessionItem {
			next := reloads[0]
			reloads = reloads[1:]
			return next
		},
	}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	_ = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("reload clearing attention scheduled a duplicate animation tick")
	}

	updated, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	_ = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("reload reintroducing attention scheduled a duplicate while previous tick was pending")
	}
}

func TestSidebarModelAttentionAnimationOffDoesNotScheduleTicks(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	if cmd := model.Init(); cmd != nil {
		t.Fatal("Init() scheduled attention animation while animation is off")
	}

	updated, cmd := model.Update(attentionAnimationTickMsg{Generation: untrackedAttentionAnimationTick})
	_ = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("Update(attentionAnimationTickMsg) scheduled attention animation while animation is off")
	}

	model = NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Slot: 1}}, Actions{
		ReloadSessions: func() []SessionItem {
			return []SessionItem{{Name: "alpha", Attention: true, Slot: 1}}
		},
	}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	updated, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF5}))
	_ = requireSidebarModel(t, updated)
	if cmd != nil {
		t.Fatal("reload scheduled attention animation while animation is off")
	}
}

func TestSidebarModelRenderAnimatesAttentionMarker(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1, Heat: string(heat.BucketStale)}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	next := model
	next.attentionAnimationFrame = 1

	first := model.Render()
	second := next.Render()
	if first == second {
		t.Fatal("animated attention marker render did not change between frames")
	}
	if !strings.Contains(stripANSI(second), attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("visible pulse frame missing bell marker in %q", stripANSI(second))
	}
}

func TestSidebarModelRenderHidesTransparentAttentionFrame(t *testing.T) {
	tests := map[string]config.AgentAttentionAnimation{
		"pulse": config.AgentAttentionAnimationPulse,
		"blink": config.AgentAttentionAnimationBlink,
	}
	for name, style := range tests {
		t.Run(name, func(t *testing.T) {
			model := NewSidebarModelWithOptions([]SessionItem{{Name: "selected"}, {Name: "alpha", Attention: true, Slot: 1, Heat: string(heat.BucketStale)}}, Actions{}, SidebarOptions{AgentAttentionAnimation: style})
			view := model.Render()
			stripped := stripANSI(view)
			if strings.Contains(stripped, attentionMarkerSymbol+" [1] alpha") {
				t.Fatalf("transparent frame rendered bell marker in %q", stripped)
			}
			if !strings.Contains(stripped, "  [1] alpha") {
				t.Fatalf("transparent frame did not preserve marker spacing in %q", stripped)
			}
			if strings.Contains(view, "38;2;0;0;0") {
				t.Fatalf("transparent frame rendered black foreground in %q", view)
			}
		})
	}
}

func TestSidebarModelRenderKeepsSelectedBackgroundForTransparentAttentionFrame(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationPulse})
	view := model.Render()
	stripped := stripANSI(view)
	if strings.Contains(stripped, attentionMarkerSymbol+" [1] alpha") {
		t.Fatalf("transparent selected frame rendered bell marker in %q", stripped)
	}
	if !strings.Contains(stripped, "  [1] alpha") {
		t.Fatalf("transparent selected frame did not preserve marker spacing in %q", stripped)
	}
	if !strings.Contains(view, "48;2;6;95;70") {
		t.Fatalf("transparent selected frame did not preserve selected background in %q", view)
	}
}

func TestSidebarModelRenderKeepsAttentionMarkerStaticWhenAnimationOff(t *testing.T) {
	model := NewSidebarModelWithOptions([]SessionItem{{Name: "alpha", Attention: true, Slot: 1, Heat: string(heat.BucketStale)}}, Actions{}, SidebarOptions{AgentAttentionAnimation: config.AgentAttentionAnimationOff})
	next := model
	next.attentionAnimationFrame = 1

	if first, second := model.Render(), next.Render(); first != second {
		t.Fatalf("off animation changed render; first=%q second=%q", first, second)
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

func TestSidebarModelHelpToggleOpensBottomSheetCheatSheet(t *testing.T) {
	model := NewSidebarModel([]SessionItem{{Name: "alpha"}}, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	model = requireSidebarModel(t, updated)
	view := model.Render()
	if !strings.Contains(view, "? keys") {
		t.Fatalf("collapsed footer missing help hint: %q", view)
	}
	if strings.Contains(view, "navigation") || strings.Contains(view, "n layout") {
		t.Fatalf("collapsed footer includes cheat sheet help: %q", view)
	}

	updated, _ = model.Update(keyPress("?", 0))
	model = requireSidebarModel(t, updated)
	view = stripANSI(model.Render())
	for _, want := range []string{"keys", "navigation", "↵ switch", "c create", "n layout", spaceKeySymbol + " pin", "h nums", "J/K", "r rename", "x kill", "esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help sheet missing %q in %q", want, view)
		}
	}
	if strings.ContainsAny(view, "╭╮╰╯│") {
		t.Fatalf("help sheet should not use side borders: %q", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = requireSidebarModel(t, updated)
	if model.showHelp {
		t.Fatal("esc should close help sheet")
	}
}

func keyPress(text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0], Mod: mod})
}

func lastRenderedLine(value string) string {
	lines := strings.Split(value, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			return lines[i]
		}
	}
	return ""
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

func TestBestEffortMetadataIconModeDefaultsToNerdWhenLocaleUnset(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	if got := bestEffortMetadataIconMode(); got != MetadataIconsNerd {
		t.Fatalf("bestEffortMetadataIconMode() = %q, want %q", got, MetadataIconsNerd)
	}
}

func TestBestEffortMetadataIconModeUsesASCIIForASCIILocale(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "C.ASCII")
	if got := bestEffortMetadataIconMode(); got != MetadataIconsASCII {
		t.Fatalf("bestEffortMetadataIconMode() = %q, want %q", got, MetadataIconsASCII)
	}
}
