package heat

import (
	"math"
	"testing"
	"time"
)

func TestDisplayForStateScalesIntensityByConfiguredRecentWindow(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	display, ok := DisplayForState(State{LastActiveAt: now.Add(-59 * time.Minute)}, now, time.Hour)
	if !ok {
		t.Fatal("DisplayForState ok = false, want true")
	}
	if display.Bucket != BucketCurrent {
		t.Fatalf("Bucket = %q, want %q", display.Bucket, BucketCurrent)
	}
	if display.Intensity >= 0.05 {
		t.Fatalf("Intensity = %f, want almost gray near the 1h window edge", display.Intensity)
	}

	display, ok = DisplayForState(State{LastActiveAt: now.Add(-time.Hour)}, now, 3*24*time.Hour)
	if !ok {
		t.Fatal("DisplayForState with 3d window ok = false, want true")
	}
	if display.Intensity <= 0.95 {
		t.Fatalf("Intensity = %f, want still hot when 1h old inside a 3d window", display.Intensity)
	}
}

func TestDisplayForStateUsesMostRecentActivitySignal(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	display, ok := DisplayForState(State{
		LastActiveAt:  now.Add(-50 * time.Minute),
		LastVisitedAt: now.Add(-10 * time.Minute),
	}, now, time.Hour)
	if !ok {
		t.Fatal("DisplayForState ok = false, want true")
	}
	if math.Abs(display.Intensity-(5.0/6.0)) > 0.001 {
		t.Fatalf("Intensity = %f, want 10m old over 1h window", display.Intensity)
	}
}

func TestDisplayByRecentActivityLimitsHighlightsToMostRecentSessions(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	states := map[string]State{
		"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
		"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
		"gamma": {LastActiveAt: now.Add(-10 * time.Minute)},
	}

	displays := DisplayByRecentActivity([]string{"alpha", "beta", "gamma"}, states, now, time.Hour, 2)

	if _, ok := displays["alpha"]; ok {
		t.Fatalf("alpha highlighted, want oldest session excluded by max-highlighted: %#v", displays)
	}
	if _, ok := displays["beta"]; !ok {
		t.Fatalf("beta missing, want most recent highlighted: %#v", displays)
	}
	if _, ok := displays["gamma"]; !ok {
		t.Fatalf("gamma missing, want second most recent highlighted: %#v", displays)
	}
}

func TestDisplayByRecentActivityAllowsUnlimitedHighlights(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	states := map[string]State{
		"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
		"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
		"gamma": {LastActiveAt: now.Add(-10 * time.Minute)},
	}

	displays := DisplayByRecentActivity([]string{"alpha", "beta", "gamma"}, states, now, time.Hour, 0)

	if len(displays) != 3 {
		t.Fatalf("highlighted count = %d, want all 3 with maxHighlighted=0: %#v", len(displays), displays)
	}
}
