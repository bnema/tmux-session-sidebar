package attention

import (
	"testing"
	"time"
)

func TestApplyEventLatchesAttentionForUnvisitedSession(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	state := ApplyEvent(State{}, Event{Action: ActionRunning, Agent: "codex", PaneID: "%1", OccurredAt: now}, false)
	state = ApplyEvent(state, Event{Action: ActionAttention, Agent: "codex", PaneID: "%1", OccurredAt: now.Add(time.Minute)}, false)

	if !state.Attention {
		t.Fatal("attention = false, want true")
	}
	if state.Panes["%1"].Active {
		t.Fatal("pane active = true, want false after attention event")
	}
}

func TestApplyEventDoesNotLatchAttentionForVisitedSession(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	state := ApplyEvent(State{Attention: true}, Event{Action: ActionAttention, Agent: "pi", PaneID: "%2", OccurredAt: now}, true)

	if state.Attention {
		t.Fatal("attention = true, want false because visit wins")
	}
	if !state.LastVisitedAt.Equal(now) {
		t.Fatalf("last visited at = %v, want %v", state.LastVisitedAt, now)
	}
}

func TestAcknowledgeVisitClearsLatchedAttention(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	state := AcknowledgeVisit(State{Attention: true}, now)

	if state.Attention {
		t.Fatal("attention = true, want false")
	}
	if !state.CurrentAcknowledged {
		t.Fatal("current acknowledged = false, want true")
	}
	if !state.LastVisitedAt.Equal(now) {
		t.Fatalf("last visited at = %v, want %v", state.LastVisitedAt, now)
	}
}

func TestApplyEventPreservesCurrentAcknowledgedAcrossRunning(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	state := ApplyEvent(State{CurrentAcknowledged: true}, Event{Action: ActionRunning, Agent: "pi", PaneID: "%2", OccurredAt: now}, false)

	if !state.CurrentAcknowledged {
		t.Fatal("current acknowledged = false, want true")
	}
	if state.Attention {
		t.Fatal("attention = true, want false")
	}
}
