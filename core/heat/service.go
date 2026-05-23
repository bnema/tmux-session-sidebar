package heat

import (
	"math"
	"time"
)

type Bucket string

type TraceStatus string

const (
	BucketCurrent Bucket = "current"
	BucketHot     Bucket = "hot"
	BucketWarm    Bucket = "warm"
	BucketCool    Bucket = "cool"
	BucketStale   Bucket = "stale"

	TraceStatusNoChange                TraceStatus = "doing-nothing"
	TraceStatusActivityDetected        TraceStatus = "activity-detected"
	TraceStatusDetectingInactivity     TraceStatus = "detecting-inactivity"
	TraceStatusAttentionStarted        TraceStatus = "attention-started"
	TraceStatusAttentionClearedOnVisit TraceStatus = "attention-cleared-on-visit"
	TraceStatusAttentionExpiredAsStale TraceStatus = "attention-expired-as-stale"
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
	Attention        bool
	Panes            map[string]PaneState
}

type Trace struct {
	Status           TraceStatus
	Bucket           Bucket
	IdleFor          time.Duration
	QuietAfter       time.Duration
	ObservedActivity bool
	Visited          bool
	Attention        bool
}

func Advance(state State, now time.Time, observedActivity bool, visited bool, halfLife time.Duration, staleAfter time.Duration, quietAfter time.Duration) (State, Trace) {
	previous := state
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
		state.Attention = false
	}
	state.UpdatedAt = now

	hasFreshUnvisitedActivity := !state.RecentActivityAt.IsZero() && state.RecentActivityAt.After(state.LastVisitedAt)
	shouldLatchAttention := !state.Attention && quietAfter > 0 && hasFreshUnvisitedActivity && now.Sub(state.RecentActivityAt) >= quietAfter

	// Attention is purely transient: stale sessions clear any latched attention, while
	// quiet-period latching only considers activity that happened after the last visit.
	// LastActiveAt feeds long-lived heat/staleness; RecentActivityAt drives quiet-after.
	switch {
	case staleAfter > 0 && !state.LastActiveAt.IsZero() && now.Sub(state.LastActiveAt) >= staleAfter:
		state.Attention = false
	case shouldLatchAttention:
		state.Attention = true
	}

	trace := Trace{
		Bucket:           BucketFor(state, now, observedActivity, halfLife, staleAfter),
		QuietAfter:       quietAfter,
		ObservedActivity: observedActivity,
		Visited:          visited,
		Attention:        state.Attention,
	}
	if !state.RecentActivityAt.IsZero() {
		trace.IdleFor = max(now.Sub(state.RecentActivityAt), 0)
	}
	// Determine trace status with intentional precedence: activity > visit-clears-attention
	// > attention transitions > inactivity detection > no change.
	switch {
	case observedActivity:
		trace.Status = TraceStatusActivityDetected
	case visited && previous.Attention:
		trace.Status = TraceStatusAttentionClearedOnVisit
	case !previous.Attention && state.Attention:
		trace.Status = TraceStatusAttentionStarted
	case previous.Attention && !state.Attention:
		trace.Status = TraceStatusAttentionExpiredAsStale
	case !state.RecentActivityAt.IsZero() && trace.IdleFor > 0 && quietAfter > 0 && trace.IdleFor < quietAfter:
		trace.Status = TraceStatusDetectingInactivity
	default:
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
	// Keep a minimum floor so very small poll intervals still register meaningful heat.
	// With the default 8h half-life, halfLife/48 is about 10 minutes of heat.
	minimum := halfLife.Seconds() / 48
	if impulse < minimum {
		return minimum
	}
	return impulse
}
