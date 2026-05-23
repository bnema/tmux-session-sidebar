package runtime

import (
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/ports"
)

const (
	testHalfLife             = 8 * time.Hour
	testStaleAfter           = 24 * time.Hour
	testAttentionQuietPeriod = 2 * time.Minute
)

func TestReconcileLiveSessionHeatRecordsPaneActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, traces := reconcileLiveSessionHeat(nil,
		[]ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
		testAttentionQuietPeriod,
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
	if state.Attention {
		t.Fatal("attention = true, want false while activity is still fresh")
	}
	if len(state.Panes) != 1 || state.Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("panes = %#v, want pane fingerprint to be persisted", state.Panes)
	}
	if bucket := heat.BucketFor(state, now, true, testHalfLife, testStaleAfter); bucket != heat.BucketCurrent {
		t.Fatalf("bucket = %q, want %q", bucket, heat.BucketCurrent)
	}
}

func TestReconcileLiveSessionHeatLatchesQuietAttentionAndClearsVisitedSessions(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	current := map[string]heat.State{
		"alpha": {
			Score:            600,
			UpdatedAt:        now,
			LastActiveAt:     now,
			RecentActivityAt: now,
			Panes:            map[string]heat.PaneState{"%1": {Fingerprint: "fp-1"}},
		},
		"beta": {
			Score:            600,
			UpdatedAt:        now,
			LastActiveAt:     now,
			RecentActivityAt: now,
			Attention:        true,
			Panes:            map[string]heat.PaneState{"%2": {Fingerprint: "fp-2"}},
		},
	}
	live := []ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}, {ID: "$2", Name: "beta"}}
	clients := []ports.TmuxClientSnapshot{{ID: "%client", CurrentSessionID: "$2", Attached: true}}
	observations := []paneObservation{
		{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true},
		{SessionID: "$2", SessionName: "beta", PaneID: "%2", Fingerprint: "fp-2", Sampled: true},
	}
	got, traces := reconcileLiveSessionHeat(current, live, clients, observations, now.Add(3*time.Minute), testHalfLife, testStaleAfter, testAttentionQuietPeriod)

	t.Run("latch attention after quiet period", func(t *testing.T) {
		if traces["alpha"].Status != heat.TraceStatusAttentionStarted {
			t.Fatalf("alpha trace status = %q, want %q", traces["alpha"].Status, heat.TraceStatusAttentionStarted)
		}
		if !got["alpha"].Attention {
			t.Fatal("alpha attention = false, want true after unseen quiet period")
		}
	})

	t.Run("clear attention on visit", func(t *testing.T) {
		if traces["beta"].Status != heat.TraceStatusAttentionClearedOnVisit {
			t.Fatalf("beta trace status = %q, want %q", traces["beta"].Status, heat.TraceStatusAttentionClearedOnVisit)
		}
		if got["beta"].Attention {
			t.Fatal("beta attention = true, want false after the user visits the session")
		}
		if !got["beta"].LastVisitedAt.Equal(now.Add(3 * time.Minute)) {
			t.Fatalf("beta last visited at = %v, want %v", got["beta"].LastVisitedAt, now.Add(3*time.Minute))
		}
	})
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

func TestDecodeHeatStateMapDropsTransientAttentionState(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	encoded := []byte(`{"Score":600,"UpdatedAt":"2026-05-23T12:00:00Z","LastActiveAt":"2026-05-23T11:55:00Z","LastVisitedAt":"2026-05-23T11:59:00Z","Attention":true,"Panes":{"%1":{"fingerprint":"abc"}}}`)

	got := decodeHeatStateMap(map[string][]byte{"alpha": encoded})["alpha"]
	if got.Score != 600 || !got.UpdatedAt.Equal(now) || !got.LastActiveAt.Equal(now.Add(-5*time.Minute)) {
		t.Fatalf("persistent heat fields changed unexpectedly: %#v", got)
	}
	if got.Attention {
		t.Fatalf("attention = true, want false after decode")
	}
	if !got.RecentActivityAt.IsZero() {
		t.Fatalf("recent activity at = %v, want zero after decode", got.RecentActivityAt)
	}
	if !got.LastVisitedAt.IsZero() {
		t.Fatalf("last visited at = %v, want zero after decode", got.LastVisitedAt)
	}
	if len(got.Panes) != 0 {
		t.Fatalf("panes = %#v, want cleared transient pane fingerprints", got.Panes)
	}
}

func TestReconcileLiveSessionHeatBootstrapsExistingStateWithoutFalseActivity(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, _ := reconcileLiveSessionHeat(
		map[string]heat.State{"alpha": {Score: 600, UpdatedAt: now, LastActiveAt: now}},
		[]ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Fingerprint: "fp-1", Sampled: true}},
		now,
		testHalfLife,
		testStaleAfter,
		testAttentionQuietPeriod,
	)

	if got["alpha"].Score != 600 {
		t.Fatalf("score = %f, want baseline bootstrap without synthetic activity", got["alpha"].Score)
	}
}

func TestReconcileLiveSessionHeatKeepsUnsampledPaneBaseline(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got, _ := reconcileLiveSessionHeat(
		map[string]heat.State{"alpha": {UpdatedAt: now, Panes: map[string]heat.PaneState{"%1": {Fingerprint: "fp-1"}}}},
		[]ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%1", Sampled: false}},
		now.Add(time.Minute),
		testHalfLife,
		testStaleAfter,
		testAttentionQuietPeriod,
	)

	if got["alpha"].Panes["%1"].Fingerprint != "fp-1" {
		t.Fatalf("pane baseline = %#v, want previous fingerprint retained on sample failure", got["alpha"].Panes)
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
		[]ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}},
		nil,
		[]paneObservation{{SessionID: "$1", SessionName: "alpha", PaneID: "%live", Fingerprint: "same", Sampled: true}},
		now.Add(time.Minute),
		testHalfLife,
		testStaleAfter,
		testAttentionQuietPeriod,
	)

	if len(got["alpha"].Panes) != 1 {
		t.Fatalf("panes = %#v, want stale pane removed", got["alpha"].Panes)
	}
	if _, ok := got["alpha"].Panes["%stale"]; ok {
		t.Fatalf("stale pane still tracked: %#v", got["alpha"].Panes)
	}
}
