package uity

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestUpdateCheckStateInitStartsImmediateCheckAndSchedulesTick(t *testing.T) {
	calls := 0
	ticks := make([]time.Duration, 0, 1)
	state := newUpdateCheckState("0.10.2", "0.10.2", false, func(current string) (bool, error) {
		calls++
		if current != "0.10.2" {
			t.Fatalf("current version = %q, want 0.10.2", current)
		}
		return false, nil
	})
	state.tick = func(interval time.Duration) tea.Cmd {
		ticks = append(ticks, interval)
		return func() tea.Msg { return updateCheckTickMsg{} }
	}

	cmd := state.initCmd()
	if cmd == nil {
		t.Fatal("initCmd returned nil")
	}
	if !state.pending {
		t.Fatal("initCmd should mark update check pending")
	}
	messages := runUpdateCheckCmd(t, cmd)
	if calls != 1 {
		t.Fatalf("check calls = %d, want 1", calls)
	}
	if len(ticks) != 1 || ticks[0] != updateCheckInterval {
		t.Fatalf("ticks = %v, want [%s]", ticks, updateCheckInterval)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want update result and tick", messages)
	}
}

func TestUpdateCheckStateTickStartsCheckAndSchedulesNextTick(t *testing.T) {
	calls := 0
	ticks := 0
	state := newUpdateCheckState("v0.10.2", "v0.10.2", false, func(string) (bool, error) {
		calls++
		return false, nil
	})
	state.pending = false
	state.tick = func(interval time.Duration) tea.Cmd {
		if interval != updateCheckInterval {
			t.Fatalf("interval = %s, want %s", interval, updateCheckInterval)
		}
		ticks++
		return func() tea.Msg { return updateCheckTickMsg{} }
	}

	state, cmd := state.handleTick()
	if cmd == nil {
		t.Fatal("handleTick returned nil")
	}
	if !state.pending {
		t.Fatal("handleTick should mark update check pending")
	}
	runUpdateCheckCmd(t, cmd)
	if calls != 1 {
		t.Fatalf("check calls = %d, want 1", calls)
	}
	if ticks != 1 {
		t.Fatalf("ticks = %d, want 1", ticks)
	}
}

func TestUpdateCheckStateTreatsSourceBuildAsAvailableWithoutRemoteCheck(t *testing.T) {
	state := newUpdateCheckState("v0.23.1+2.gabc1234*", "", true, func(string) (bool, error) {
		t.Fatal("source build should not run remote update check")
		return false, nil
	})
	if !state.available {
		t.Fatal("source build should show update indicator")
	}
	if cmd := state.initCmd(); cmd != nil {
		t.Fatal("source build should not schedule update check")
	}
}

func TestUpdateCheckStateUsesReleaseCheckVersionForRemoteCheck(t *testing.T) {
	state := newUpdateCheckState("v0.23.1+2.gabc1234*", "v0.23.1", false, func(current string) (bool, error) {
		if current != "v0.23.1" {
			t.Fatalf("current version = %q, want release check version", current)
		}
		return false, nil
	})
	state.pending = false
	if _, cmd := state.startCheckCmd(); cmd == nil {
		t.Fatal("release build should schedule update check")
	} else {
		runUpdateCheckCmd(t, cmd)
	}
}

func TestUpdateCheckStateStopsWhenNotCheckableOrAvailable(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		available bool
		check     func(string) (bool, error)
	}{
		{name: "source version", version: "source", check: func(string) (bool, error) { t.Fatal("check should not run"); return true, nil }},
		{name: "empty version", version: "", check: func(string) (bool, error) { t.Fatal("check should not run"); return true, nil }},
		{name: "nil checker", version: "0.10.2"},
		{name: "already available", version: "0.10.2", available: true, check: func(string) (bool, error) { t.Fatal("check should not run"); return true, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newUpdateCheckState(tt.version, tt.version, false, tt.check)
			state.available = tt.available
			state.tick = func(time.Duration) tea.Cmd {
				t.Fatal("tick should not be scheduled")
				return nil
			}
			if cmd := state.initCmd(); cmd != nil {
				t.Fatalf("initCmd returned command for %s", tt.name)
			}
			if _, cmd := state.handleTick(); cmd != nil {
				t.Fatalf("handleTick returned command for %s", tt.name)
			}
		})
	}
}

func TestUpdateCheckStateStopsAfterUpdateFound(t *testing.T) {
	state := newUpdateCheckState("0.10.2", "0.10.2", false, func(string) (bool, error) {
		return true, nil
	})
	state.pending = false
	state, cmd := state.startCheckCmd()
	if cmd == nil {
		t.Fatal("startCheckCmd returned nil")
	}
	messages := runUpdateCheckCmd(t, cmd)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one update result", messages)
	}
	msg, ok := messages[0].(updateAvailableMsg)
	if !ok || !msg.available {
		t.Fatalf("message = %#v, want updateAvailableMsg true", messages[0])
	}
	state = state.handleResult(msg)
	if !state.available {
		t.Fatal("state should be available after true result")
	}
	state.tick = func(time.Duration) tea.Cmd {
		t.Fatal("tick should not be scheduled after update found")
		return nil
	}
	if _, cmd := state.handleTick(); cmd != nil {
		t.Fatal("handleTick returned command after update found")
	}
}

func TestUpdateCheckStateDoesNotOverlapChecks(t *testing.T) {
	calls := 0
	ticks := 0
	state := newUpdateCheckState("0.10.2", "0.10.2", false, func(string) (bool, error) {
		calls++
		return false, nil
	})
	state.pending = true
	state.tick = func(time.Duration) tea.Cmd {
		ticks++
		return func() tea.Msg { return updateCheckTickMsg{} }
	}

	state, cmd := state.handleTick()
	if cmd == nil {
		t.Fatal("handleTick should still schedule the next tick while pending")
	}
	if !state.pending {
		t.Fatal("state should remain pending")
	}
	runUpdateCheckCmd(t, cmd)
	if calls != 0 {
		t.Fatalf("check calls = %d, want 0", calls)
	}
	if ticks != 1 {
		t.Fatalf("ticks = %d, want 1", ticks)
	}
}

func TestUpdateCheckStateClearsPendingOnError(t *testing.T) {
	state := newUpdateCheckState("0.10.2", "0.10.2", false, func(string) (bool, error) {
		return false, errors.New("network")
	})
	state.pending = false
	state, cmd := state.startCheckCmd()
	if cmd == nil {
		t.Fatal("startCheckCmd returned nil")
	}
	messages := runUpdateCheckCmd(t, cmd)
	msg, ok := messages[0].(updateAvailableMsg)
	if !ok || msg.available {
		t.Fatalf("message = %#v, want unavailable result", messages[0])
	}
	state = state.handleResult(msg)
	if state.pending {
		t.Fatal("pending should be cleared after error result")
	}
}

func runUpdateCheckCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		messages := make([]tea.Msg, 0, len(batch))
		for _, child := range batch {
			messages = append(messages, child())
		}
		return messages
	}
	return []tea.Msg{msg}
}
