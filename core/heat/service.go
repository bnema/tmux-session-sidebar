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
	TraceStatusAttentionStarted        TraceStatus = "attention-started"
	TraceStatusAttentionClearedOnVisit TraceStatus = "attention-cleared-on-visit"
	TraceStatusAttentionExpiredAsStale TraceStatus = "attention-expired-as-stale"
)

type AgentPhase string

const (
	AgentPhaseRunning   AgentPhase = "running"
	AgentPhaseCompleted AgentPhase = "completed"
)

type PaneState struct {
	Fingerprint string     `json:"fingerprint,omitempty"`
	AgentKind   string     `json:"agentKind,omitempty"`
	AgentPhase  AgentPhase `json:"agentPhase,omitempty"`
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
	ObservedActivity bool
	Visited          bool
	Attention        bool
}

func Advance(state State, now time.Time, observedActivity bool, observedAgentCompletion bool, visited bool, halfLife time.Duration, staleAfter time.Duration) (State, Trace) {
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

	// Attention is explicit: terminal output keeps heat fresh, but the bell only
	// rings when the runtime observes an agent completion/notification signal.
	// A visit is the user's acknowledgement, so it wins over completion observed
	// during the same sampling tick. Otherwise completion wins over stale heat.
	switch {
	case visited:
		state.Attention = false
	case observedAgentCompletion:
		state.Attention = true
	case staleAfter > 0 && !state.LastActiveAt.IsZero() && now.Sub(state.LastActiveAt) >= staleAfter:
		state.Attention = false
	}

	trace := Trace{
		Bucket:           BucketFor(state, now, observedActivity, halfLife, staleAfter),
		ObservedActivity: observedActivity,
		Visited:          visited,
		Attention:        state.Attention,
	}
	if !state.RecentActivityAt.IsZero() {
		trace.IdleFor = max(now.Sub(state.RecentActivityAt), 0)
	}
	// Determine trace status with intentional precedence: visit clears first, then
	// agent completion, activity, expiration, and no change.
	switch {
	case visited && previous.Attention:
		trace.Status = TraceStatusAttentionClearedOnVisit
	case observedAgentCompletion && !previous.Attention && state.Attention:
		trace.Status = TraceStatusAttentionStarted
	case observedActivity:
		trace.Status = TraceStatusActivityDetected
	case previous.Attention && !state.Attention:
		trace.Status = TraceStatusAttentionExpiredAsStale
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
