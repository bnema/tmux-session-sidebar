package heat

import (
	"math"
	"time"
)

type Bucket string

const (
	BucketCurrent Bucket = "current"
	BucketHot     Bucket = "hot"
	BucketWarm    Bucket = "warm"
	BucketCool    Bucket = "cool"
	BucketStale   Bucket = "stale"
)

type State struct {
	Score         float64
	UpdatedAt     time.Time
	LastSeenAt    time.Time
	AttachedCount int
}

func Advance(state State, now time.Time, attachedCount int, halfLife time.Duration, staleAfter time.Duration) (State, Bucket) {
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = now
	}
	if halfLife <= 0 {
		halfLife = 8 * time.Hour
	}
	if attachedCount < 0 {
		attachedCount = 0
	}

	elapsed := now.Sub(state.UpdatedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedSeconds := elapsed.Seconds()
	decay := math.Pow(0.5, elapsedSeconds/halfLife.Seconds())
	state.Score *= decay
	if state.AttachedCount > 0 {
		state.Score += elapsedSeconds
	}

	state.UpdatedAt = now
	if state.AttachedCount > 0 || attachedCount > 0 {
		state.LastSeenAt = now
	}
	state.AttachedCount = attachedCount

	return state, BucketFor(state, now, attachedCount > 0, halfLife, staleAfter)
}

func BucketFor(state State, now time.Time, current bool, halfLife time.Duration, staleAfter time.Duration) Bucket {
	if current {
		return BucketCurrent
	}
	if staleAfter > 0 && !state.LastSeenAt.IsZero() && now.Sub(state.LastSeenAt) >= staleAfter {
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
