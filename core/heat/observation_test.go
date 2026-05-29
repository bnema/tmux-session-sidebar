package heat

import "testing"

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
