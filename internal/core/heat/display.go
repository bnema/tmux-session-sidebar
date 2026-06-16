package heat

import (
	"slices"
	"time"
)

type Display struct {
	Bucket    Bucket
	Intensity float64
	Age       time.Duration
}

func DisplayForState(state State, now time.Time, recentWindow time.Duration) (Display, bool) {
	age, ok := RecentActivityAge(state, now, recentWindow)
	if !ok {
		return Display{Bucket: BucketStale}, false
	}
	return Display{Bucket: BucketCurrent, Intensity: IntensityForAge(age, recentWindow), Age: age}, true
}

func DisplayByRecentActivity(names []string, states map[string]State, now time.Time, recentWindow time.Duration, maxHighlighted int) map[string]Display {
	type candidate struct {
		name    string
		display Display
	}
	candidates := make([]candidate, 0, len(names))
	for _, name := range names {
		state, ok := states[name]
		if !ok {
			continue
		}
		display, ok := DisplayForState(state, now, recentWindow)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{name: name, display: display})
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		switch {
		case a.display.Age < b.display.Age:
			return -1
		case a.display.Age > b.display.Age:
			return 1
		default:
			return 0
		}
	})
	if maxHighlighted > 0 && len(candidates) > maxHighlighted {
		candidates = candidates[:maxHighlighted]
	}
	displays := make(map[string]Display, len(candidates))
	for _, candidate := range candidates {
		displays[candidate.name] = candidate.display
	}
	return displays
}

func RecentActivityAge(state State, now time.Time, window time.Duration) (time.Duration, bool) {
	if window <= 0 {
		window = time.Hour
	}
	var mostRecent time.Time
	for _, timestamp := range []time.Time{state.LastVisitedAt, state.LastActiveAt} {
		if timestamp.IsZero() || timestamp.After(now) {
			continue
		}
		if mostRecent.IsZero() || timestamp.After(mostRecent) {
			mostRecent = timestamp
		}
	}
	if mostRecent.IsZero() {
		return 0, false
	}
	age := now.Sub(mostRecent)
	if age > window {
		return 0, false
	}
	return age, true
}

func IntensityForAge(age time.Duration, window time.Duration) float64 {
	if window <= 0 || age <= 0 {
		return 1
	}
	if age >= window {
		return 0
	}
	return 1 - age.Seconds()/window.Seconds()
}
