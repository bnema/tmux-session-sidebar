package heat

import (
	"testing"
	"time"
)

func TestApplyPaneObservationsUsesFirstSampleAsBaseline(t *testing.T) {
	state := State{}

	active := ApplyPaneObservations(&state, []PaneObservation{{PaneID: "%1", Fingerprint: "fp-1", Sampled: true}})

	if active {
		t.Fatal("active = true, want false for first sample baseline")
	}
	if state.Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("panes = %#v, want first sample persisted as baseline", state.Panes)
	}
}

func TestApplyPaneObservationsReportsChangedFingerprintAsActivity(t *testing.T) {
	state := State{Panes: map[string]PaneState{"%1": {Fingerprint: "fp-1"}}}

	active := ApplyPaneObservations(&state, []PaneObservation{{PaneID: "%1", Fingerprint: "fp-2", Sampled: true}})

	if !active {
		t.Fatal("active = false, want true after fingerprint changed")
	}
	if state.Panes["%1"].Fingerprint != "fp-2" {
		t.Fatalf("panes = %#v, want changed fingerprint persisted", state.Panes)
	}
}

func TestDisplayBucketUsesRecentActivityWindow(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	if got := DisplayBucket(State{LastActiveAt: now.Add(-30 * time.Minute)}, now, time.Hour); got != BucketCurrent {
		t.Fatalf("DisplayBucket recent active = %q, want %q", got, BucketCurrent)
	}
	if got := DisplayBucket(State{LastVisitedAt: now.Add(-2 * time.Hour)}, now, time.Hour); got != BucketStale {
		t.Fatalf("DisplayBucket old visit = %q, want %q", got, BucketStale)
	}
}
