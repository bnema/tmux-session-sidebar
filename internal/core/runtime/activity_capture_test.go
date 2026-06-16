package runtime

import (
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

const (
	testHalfLife   = 8 * time.Hour
	testStaleAfter = 24 * time.Hour
)

func TestReconcileLiveSessionHeatDoesNotTreatFirstPaneObservationAsActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, traces := reconcileLiveSessionHeat(nil,
		[]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
	)

	state, ok := got["alpha"]
	if !ok {
		t.Fatalf("missing heat state for alpha: %#v", got)
	}
	if traces["alpha"].Status != heat.TraceStatusNoChange {
		t.Fatalf("trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusNoChange)
	}
	if !state.LastActiveAt.IsZero() {
		t.Fatalf("last active at = %v, want zero for first observation baseline", state.LastActiveAt)
	}
	if len(state.Panes) != 1 || state.Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("panes = %#v, want pane fingerprint to be persisted as baseline", state.Panes)
	}
}

func TestReconcileLiveSessionHeatRecordsPaneActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	current := map[string]heat.State{
		"alpha": {UpdatedAt: now.Add(-time.Minute), Panes: map[string]heat.PaneState{"%1": {Fingerprint: "fp-0"}}},
	}
	got, traces := reconcileLiveSessionHeat(current,
		[]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
	)

	state, ok := got["alpha"]
	if !ok {
		t.Fatalf("missing heat state for alpha: %#v", got)
	}
	if traces["alpha"].Status != heat.TraceStatusActivityDetected {
		t.Fatalf("trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusActivityDetected)
	}
	if !state.LastActiveAt.Equal(now) {
		t.Fatalf("last active at = %v, want %v", state.LastActiveAt, now)
	}
	if len(state.Panes) != 1 || state.Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("panes = %#v, want pane fingerprint to be persisted", state.Panes)
	}
	if bucket := heat.BucketFor(state, now, true, testHalfLife, testStaleAfter); bucket != heat.BucketCurrent {
		t.Fatalf("bucket = %q, want %q", bucket, heat.BucketCurrent)
	}
}

func TestReconcileLiveSessionHeatDoesNotTreatQuietOutputAsActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	current := map[string]heat.State{
		"alpha": {
			Score:            600,
			UpdatedAt:        now,
			LastActiveAt:     now,
			RecentActivityAt: now,
			Panes:            map[string]heat.PaneState{"%1": {Fingerprint: "fp-1"}},
		},
	}
	live := []ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}
	observations := []paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}}

	got, traces := reconcileLiveSessionHeat(current, live, nil, observations, now.Add(3*time.Minute), testHalfLife, testStaleAfter)

	if traces["alpha"].Status != heat.TraceStatusNoChange {
		t.Fatalf("trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusNoChange)
	}
	if !got["alpha"].RecentActivityAt.Equal(now) {
		t.Fatalf("recent activity at = %v, want %v", got["alpha"].RecentActivityAt, now)
	}
}

func TestReconcileLiveSessionHeatUpdatesVisitSignal(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	current := map[string]heat.State{
		"beta": {
			Score:            600,
			UpdatedAt:        now,
			LastActiveAt:     now,
			RecentActivityAt: now,
			Panes:            map[string]heat.PaneState{"%2": {Fingerprint: "fp-2"}},
		},
	}
	live := []ports.SessionSnapshot{{ID: "$2", Name: "beta"}}
	clients := []ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$2", Attached: true}}
	observations := []paneObservation{{SessionID: "$2", SessionName: "beta", PaneID: "%2", Fingerprint: "fp-2", Sampled: true}}

	got, traces := reconcileLiveSessionHeat(current, live, clients, observations, now.Add(3*time.Minute), testHalfLife, testStaleAfter)

	if traces["beta"].Visited != true {
		t.Fatal("visited = false, want true")
	}
	if !got["beta"].LastVisitedAt.Equal(now.Add(3 * time.Minute)) {
		t.Fatalf("last visited at = %v, want %v", got["beta"].LastVisitedAt, now.Add(3*time.Minute))
	}
}

func TestDecodeHeatStateMapKeepsLegacyScoreWithoutSynthesizingNewFields(t *testing.T) {
	legacy := []byte(`{"Score":7200,"UpdatedAt":"2026-05-17T12:00:00Z","LastSeenAt":"2026-05-17T11:30:00Z","AttachedCount":1}`)

	got := decodeHeatStateMap(map[string][]byte{"alpha": legacy})
	if got["alpha"].Score != 7200 {
		t.Fatalf("score = %f, want 7200 (legacy score retained)", got["alpha"].Score)
	}
	if !got["alpha"].LastActiveAt.IsZero() || !got["alpha"].LastVisitedAt.IsZero() {
		t.Fatalf("legacy decode synthesized fields unexpectedly: %#v (want zero LastActiveAt and LastVisitedAt)", got["alpha"])
	}
}

func TestDecodeHeatStateMapPreservesTransientHeatStateBetweenTicks(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	encoded := encodeHeatStateMap(map[string]heat.State{"alpha": {
		Score:            600,
		UpdatedAt:        now,
		LastActiveAt:     now.Add(-5 * time.Minute),
		RecentActivityAt: now.Add(-time.Minute),
		LastVisitedAt:    now.Add(-2 * time.Minute),
		Panes:            map[string]heat.PaneState{"%1": {Fingerprint: "abc"}},
	}})

	got := decodeHeatStateMap(encoded)["alpha"]
	if got.Score != 600 || !got.UpdatedAt.Equal(now) || !got.LastActiveAt.Equal(now.Add(-5*time.Minute)) {
		t.Fatalf("persistent heat fields changed unexpectedly: %#v", got)
	}
	if !got.RecentActivityAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("recent activity at = %v, want %v", got.RecentActivityAt, now.Add(-time.Minute))
	}
	if !got.LastVisitedAt.Equal(now.Add(-2 * time.Minute)) {
		t.Fatalf("last visited at = %v, want %v", got.LastVisitedAt, now.Add(-2*time.Minute))
	}
	if got.Panes["%1"].Fingerprint != "abc" {
		t.Fatalf("panes = %#v, want persisted transient pane fingerprints between ticks", got.Panes)
	}
}

func TestReconcileLiveSessionHeatBootstrapsExistingStateWithoutFalseActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, _ := reconcileLiveSessionHeat(
		map[string]heat.State{"alpha": {Score: 600, UpdatedAt: now, LastActiveAt: now}},
		[]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
	)

	if got["alpha"].Score != 600 {
		t.Fatalf("score = %f, want baseline bootstrap without synthetic activity", got["alpha"].Score)
	}
}

func TestReconcileLiveSessionHeatKeepsUnsampledPaneBaseline(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, _ := reconcileLiveSessionHeat(
		map[string]heat.State{"alpha": {UpdatedAt: now, Panes: map[string]heat.PaneState{"%1": {Fingerprint: "fp-1"}}}},
		[]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Sampled: false}},
		now.Add(time.Minute),
		testHalfLife,
		testStaleAfter,
	)

	if got["alpha"].Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("pane baseline = %#v, want previous fingerprint retained on sample failure", got["alpha"].Panes)
	}
}

func TestReconcileLiveSessionHeatContinuesTrackingPaneChangesAcrossPersistedTicks(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	live := []ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}

	first, _ := reconcileLiveSessionHeat(
		nil,
		live,
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
	)
	persisted := encodeHeatStateMap(first)

	next, traces := reconcileLiveSessionHeat(
		decodeHeatStateMap(persisted),
		live,
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-2", Sampled: true}},
		now.Add(5*time.Second),
		testHalfLife,
		testStaleAfter,
	)

	if traces["alpha"].Status != heat.TraceStatusActivityDetected {
		t.Fatalf("trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusActivityDetected)
	}
	if !next["alpha"].LastActiveAt.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("last active at = %v, want %v", next["alpha"].LastActiveAt, now.Add(5*time.Second))
	}
	if got := next["alpha"].Panes["%1"].Fingerprint; got != "fp-2" {
		t.Fatalf("pane fingerprint = %q, want fp-2", got)
	}
}

func TestReconcileLiveSessionHeatDoesNotPreserveQuietTimerAcrossPersistedTicks(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	live := []ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}

	first, _ := reconcileLiveSessionHeat(
		nil,
		live,
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
	)
	persisted := encodeHeatStateMap(first)

	next, traces := reconcileLiveSessionHeat(
		decodeHeatStateMap(persisted),
		live,
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now.Add(3*time.Minute),
		testHalfLife,
		testStaleAfter,
	)

	if traces["alpha"].Status != heat.TraceStatusNoChange {
		t.Fatalf("trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusNoChange)
	}
	if !next["alpha"].RecentActivityAt.IsZero() {
		t.Fatalf("recent activity at = %v, want zero because first sample was only a baseline", next["alpha"].RecentActivityAt)
	}
}

func TestReconcileLiveSessionHeatPrunesMissingPaneFingerprints(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	current := map[string]heat.State{
		"alpha": {
			UpdatedAt: now,
			Panes: map[string]heat.PaneState{
				"%stale": {Fingerprint: "old"},
				"%live":  {Fingerprint: "same"},
			},
		},
	}

	got, _ := reconcileLiveSessionHeat(current,
		[]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%live", Fingerprint: "same", Sampled: true}},
		now.Add(time.Minute),
		testHalfLife,
		testStaleAfter,
	)

	if len(got["alpha"].Panes) != 1 {
		t.Fatalf("panes = %#v, want stale pane removed", got["alpha"].Panes)
	}
	if _, ok := got["alpha"].Panes["%stale"]; ok {
		t.Fatalf("stale pane still tracked: %#v", got["alpha"].Panes)
	}
}
