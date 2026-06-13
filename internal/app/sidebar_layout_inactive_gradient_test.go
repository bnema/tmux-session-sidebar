package app

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSessionItemsFromStateAssignsInactiveIntensityByActualRecency(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"gamma", "alpha", "beta", "hot", "unknown"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
			"beta":  {LastVisitedAt: now.Add(-12 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-24 * time.Hour)},
			"hot":   {LastActiveAt: now.Add(-10 * time.Minute)},
		}),
	}
	views := []sessions.View{
		{Name: "alpha", Visible: true},
		{Name: "beta", Visible: true},
		{Name: "gamma", Visible: true},
		{Name: "hot", Visible: true},
		{Name: "unknown", Visible: true},
	}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour})

	if got := byName["alpha"].InactiveIntensity; got != 0 {
		t.Fatalf("alpha inactive intensity = %v, want oldest stale session at dark endpoint", got)
	}
	if got := byName["gamma"].InactiveIntensity; math.Abs(got-0.5) > 0.001 {
		t.Fatalf("gamma inactive intensity = %v, want midpoint stale shade", got)
	}
	if got := byName["beta"].InactiveIntensity; got != 1 {
		t.Fatalf("beta inactive intensity = %v, want freshest stale session at light endpoint", got)
	}
	if got := byName["hot"].Heat; got != string(heat.BucketCurrent) {
		t.Fatalf("hot heat bucket = %q, want current heat bucket", got)
	}
	if got := byName["hot"].InactiveIntensity; got != 0 {
		t.Fatalf("hot inactive intensity = %v, want heat-highlighted session excluded from inactive gradient", got)
	}
	if got := byName["unknown"].InactiveIntensity; got != 0 {
		t.Fatalf("unknown inactive intensity = %v, want sessions without activity timestamps to stay at dark endpoint", got)
	}
}

func TestSessionItemsFromStateGivesEqualRecencyTiesEqualInactiveIntensity(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "gamma"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
			"beta":  {LastVisitedAt: now.Add(-48 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-12 * time.Hour)},
		}),
	}
	views := []sessions.View{
		{Name: "alpha", Visible: true},
		{Name: "beta", Visible: true},
		{Name: "gamma", Visible: true},
	}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour})

	if got := byName["alpha"].InactiveIntensity; got != 0 {
		t.Fatalf("alpha inactive intensity = %v, want dark endpoint for oldest tie group", got)
	}
	if got := byName["beta"].InactiveIntensity; got != 0 {
		t.Fatalf("beta inactive intensity = %v, want same dark endpoint as alpha for equal recency", got)
	}
	if got := byName["gamma"].InactiveIntensity; got != 1 {
		t.Fatalf("gamma inactive intensity = %v, want freshest distinct group at light endpoint", got)
	}
}

func encodeHeatStateMapForSidebarGradientTest(states map[string]heat.State) map[string][]byte {
	encoded := make(map[string][]byte, len(states))
	for name, state := range states {
		data, err := json.Marshal(state)
		if err != nil {
			panic(err)
		}
		encoded[name] = data
	}
	return encoded
}
