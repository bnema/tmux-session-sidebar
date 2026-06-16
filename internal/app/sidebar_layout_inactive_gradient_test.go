package app

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
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

func TestSessionItemsFromStateUsesVisibleOrderToBreakInactiveRecencyTies(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "gamma", "delta"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
			"beta":  {LastVisitedAt: now.Add(-48 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-48 * time.Hour)},
			"delta": {LastVisitedAt: now.Add(-48 * time.Hour)},
		}),
	}
	views := []sessions.View{
		{Name: "alpha", Visible: true},
		{Name: "beta", Visible: true},
		{Name: "gamma", Visible: true},
		{Name: "delta", Visible: true},
	}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour})

	if got := byName["alpha"].InactiveIntensity; got != 1 {
		t.Fatalf("alpha inactive intensity = %v, want first visible tied session at light endpoint", got)
	}
	if got := byName["beta"].InactiveIntensity; math.Abs(got-(2.0/3.0)) > 0.001 {
		t.Fatalf("beta inactive intensity = %v, want second visible tied session stepped down from alpha", got)
	}
	if got := byName["gamma"].InactiveIntensity; math.Abs(got-(1.0/3.0)) > 0.001 {
		t.Fatalf("gamma inactive intensity = %v, want third visible tied session stepped down again", got)
	}
	if got := byName["delta"].InactiveIntensity; got != 0 {
		t.Fatalf("delta inactive intensity = %v, want last visible tied session at dark endpoint", got)
	}
}

func TestSessionItemsFromStateIgnoresHiddenNumericSessionsWhenSpreadingInactiveGradient(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "1", "beta", "2", "gamma"},
		Sidebar:      &ports.SidebarState{ShowNumericSessions: false},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-72 * time.Hour)},
			"1":     {LastActiveAt: now.Add(-60 * time.Hour)},
			"beta":  {LastActiveAt: now.Add(-48 * time.Hour)},
			"2":     {LastActiveAt: now.Add(-36 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-24 * time.Hour)},
		}),
	}
	views := []sessions.View{
		{Name: "alpha", Visible: true},
		{Name: "1", Visible: true},
		{Name: "beta", Visible: true},
		{Name: "2", Visible: true},
		{Name: "gamma", Visible: true},
	}

	items, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour})

	if got := len(items); got != 3 {
		t.Fatalf("visible session item count = %d, want hidden numeric sessions excluded", got)
	}
	if _, exists := byName["1"]; exists {
		t.Fatalf("byName unexpectedly contains hidden numeric session 1")
	}
	if _, exists := byName["2"]; exists {
		t.Fatalf("byName unexpectedly contains hidden numeric session 2")
	}
	if got := byName["alpha"].InactiveIntensity; got != 0 {
		t.Fatalf("alpha inactive intensity = %v, want oldest visible stale session at dark endpoint", got)
	}
	if got := byName["beta"].InactiveIntensity; math.Abs(got-0.5) > 0.001 {
		t.Fatalf("beta inactive intensity = %v, want midpoint visible stale shade", got)
	}
	if got := byName["gamma"].InactiveIntensity; got != 1 {
		t.Fatalf("gamma inactive intensity = %v, want freshest visible stale session at light endpoint", got)
	}
}

func TestSessionItemsFromStateIncludesRecentSessionsInInactiveGradientWhenHeatColorsDisabled(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "hot"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
			"beta":  {LastVisitedAt: now.Add(-24 * time.Hour)},
			"hot":   {LastActiveAt: now.Add(-5 * time.Minute)},
		}),
	}
	views := []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}, {Name: "hot", Visible: true}}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: false, HeatRecentInterval: time.Hour})

	if got := byName["hot"].Heat; got != "" {
		t.Fatalf("hot heat bucket = %q, want no explicit heat when colors are disabled", got)
	}
	if got := byName["alpha"].InactiveIntensity; got != 0 {
		t.Fatalf("alpha inactive intensity = %v, want oldest stale session at dark endpoint", got)
	}
	if got := byName["beta"].InactiveIntensity; math.Abs(got-0.5) > 0.001 {
		t.Fatalf("beta inactive intensity = %v, want midpoint stale shade", got)
	}
	if got := byName["hot"].InactiveIntensity; got != 1 {
		t.Fatalf("hot inactive intensity = %v, want freshest session included in inactive gradient when heat is disabled", got)
	}
}

func TestSessionItemsFromStateKeepsNonHighlightedRecentSessionsInInactiveGradient(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "warm", "hot"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
			"beta":  {LastVisitedAt: now.Add(-24 * time.Hour)},
			"warm":  {LastActiveAt: now.Add(-10 * time.Minute)},
			"hot":   {LastActiveAt: now.Add(-5 * time.Minute)},
		}),
	}
	views := []sessions.View{{Name: "alpha", Visible: true}, {Name: "beta", Visible: true}, {Name: "warm", Visible: true}, {Name: "hot", Visible: true}}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour, HeatMaxHighlighted: 1})

	if got := byName["hot"].Heat; got != string(heat.BucketCurrent) {
		t.Fatalf("hot heat bucket = %q, want hottest session highlighted", got)
	}
	if got := byName["hot"].InactiveIntensity; got != 0 {
		t.Fatalf("hot inactive intensity = %v, want highlighted hottest session excluded from inactive gradient", got)
	}
	if got := byName["warm"].Heat; got != "" {
		t.Fatalf("warm heat bucket = %q, want capped recent session left unhighlighted", got)
	}
	if got := byName["alpha"].InactiveIntensity; got != 0 {
		t.Fatalf("alpha inactive intensity = %v, want oldest non-highlighted session at dark endpoint", got)
	}
	if got := byName["beta"].InactiveIntensity; math.Abs(got-0.5) > 0.001 {
		t.Fatalf("beta inactive intensity = %v, want midpoint non-highlighted shade", got)
	}
	if got := byName["warm"].InactiveIntensity; got != 1 {
		t.Fatalf("warm inactive intensity = %v, want freshest non-highlighted session at light endpoint", got)
	}
}

func TestSessionItemsFromStateUsesLightEndpointForSingleInactiveCandidate(t *testing.T) {
	now := time.Now().UTC()
	persisted := ports.PersistedState{
		SessionOrder: []string{"alpha", "unknown"},
		Heat: encodeHeatStateMapForSidebarGradientTest(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-48 * time.Hour)},
		}),
	}
	views := []sessions.View{{Name: "alpha", Visible: true}, {Name: "unknown", Visible: true}}

	_, byName := sessionItemsFromState("current", views, persisted, ports.ConfigSnapshot{HeatColorsEnabled: true, HeatRecentInterval: time.Hour})

	if got := byName["alpha"].InactiveIntensity; got != 1 {
		t.Fatalf("alpha inactive intensity = %v, want lone inactive candidate at light endpoint", got)
	}
	if got := byName["unknown"].InactiveIntensity; got != 0 {
		t.Fatalf("unknown inactive intensity = %v, want sessions without recency signals unchanged", got)
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
