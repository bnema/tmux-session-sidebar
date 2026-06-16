package heat

import (
	"math"
	"time"
)

type Bucket string

type TraceStatus string

const (
	BucketCurrent Bucket = "current"
	// Hot, warm, and cool buckets describe persisted heat traces. Display heat uses
	// a continuous intensity instead of these discrete non-stale buckets.
	BucketHot   Bucket = "hot"
	BucketWarm  Bucket = "warm"
	BucketCool  Bucket = "cool"
	BucketStale Bucket = "stale"

	TraceStatusNoChange         TraceStatus = "doing-nothing"
	TraceStatusActivityDetected TraceStatus = "activity-detected"
)

type PaneState struct {
	Fingerprint string `json:"fingerprint,omitempty"`
}

type State struct {
	Score            float64
	UpdatedAt        time.Time
	LastActiveAt     time.Time
	RecentActivityAt time.Time
	LastVisitedAt    time.Time
	Panes            map[string]PaneState
}

type Trace struct {
	Status           TraceStatus
	Bucket           Bucket
	IdleFor          time.Duration
	ObservedActivity bool
	Visited          bool
}

func Advance(state State, now time.Time, observedActivity bool, visited bool, halfLife time.Duration, staleAfter time.Duration) (State, Trace) {
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = now
	}
	if halfLife <= 0 {
		halfLife = 8 * time.Hour
	}
	if state.Panes == nil {
		state.Panes = map[string]PaneState{}
	}

	elapsed := max(now.Sub(state.UpdatedAt), 0)
	state.Score *= decayFactor(elapsed, halfLife)
	if observedActivity {
		state.Score += activityImpulse(elapsed, halfLife)
		state.LastActiveAt = now
		state.RecentActivityAt = now
	}
	if visited {
		state.LastVisitedAt = now
	}
	state.UpdatedAt = now

	trace := Trace{
		Bucket:           BucketFor(state, now, observedActivity, halfLife, staleAfter),
		ObservedActivity: observedActivity,
		Visited:          visited,
	}
	if !state.RecentActivityAt.IsZero() {
		trace.IdleFor = max(now.Sub(state.RecentActivityAt), 0)
	}
	if observedActivity {
		trace.Status = TraceStatusActivityDetected
	} else {
		trace.Status = TraceStatusNoChange
	}

	return state, trace
}

func BucketFor(state State, now time.Time, active bool, halfLife time.Duration, staleAfter time.Duration) Bucket {
	if active {
		return BucketCurrent
	}
	if staleAfter > 0 && !state.LastActiveAt.IsZero() && now.Sub(state.LastActiveAt) >= staleAfter {
		return BucketStale
	}
	if halfLife <= 0 {
		halfLife = 8 * time.Hour
	}
	halfLifeSeconds := halfLife.Seconds()
	switch {
	case state.Score >= halfLifeSeconds/4:
		return BucketHot
	case state.Score >= halfLifeSeconds/12:
		return BucketWarm
	default:
		return BucketCool
	}
}

func decayFactor(elapsed time.Duration, halfLife time.Duration) float64 {
	if elapsed <= 0 {
		return 1
	}
	return math.Pow(0.5, elapsed.Seconds()/halfLife.Seconds())
}

func activityImpulse(elapsed time.Duration, halfLife time.Duration) float64 {
	impulse := elapsed.Seconds()
	minimum := halfLife.Seconds() / 48
	if impulse < minimum {
		return minimum
	}
	return impulse
}
