package heat

import (
	"testing"
	"time"
)

func TestAdvanceTraceReportsActivityAndIdleState(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	_, trace := Advance(State{}, now, true, false, 8*time.Hour, 24*time.Hour)
	if trace.Status != TraceStatusActivityDetected {
		t.Fatalf("activity status = %q, want %q", trace.Status, TraceStatusActivityDetected)
	}

	state := State{Score: 600, UpdatedAt: now, LastActiveAt: now, RecentActivityAt: now}
	_, trace = Advance(state, now.Add(90*time.Second), false, false, 8*time.Hour, 24*time.Hour)
	if trace.Status != TraceStatusNoChange {
		t.Fatalf("quiet status = %q, want %q", trace.Status, TraceStatusNoChange)
	}
	if trace.IdleFor != 90*time.Second {
		t.Fatalf("idle for = %s, want 90s", trace.IdleFor)
	}

	_, trace = Advance(state, now.Add(2*time.Minute), false, true, 8*time.Hour, 24*time.Hour)
	if !trace.Visited {
		t.Fatal("visited = false, want true")
	}
	if trace.Status != TraceStatusNoChange {
		t.Fatalf("visit status = %q, want %q", trace.Status, TraceStatusNoChange)
	}
}
