package heat

import (
	"testing"
	"time"
)

func TestAdvanceHeat(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		state         State
		now           time.Time
		attachedCount int
		halfLife      time.Duration
		staleAfter    time.Duration
		wantBucket    Bucket
		wantMin       float64
		wantMax       float64
		wantAttached  int
	}{
		{name: "current attached", state: State{UpdatedAt: now.Add(-2 * time.Hour), AttachedCount: 1}, now: now, attachedCount: 1, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketCurrent, wantMin: 7199, wantMax: 7201, wantAttached: 1},
		{name: "hot detached", state: State{Score: 7200, UpdatedAt: now, LastSeenAt: now}, now: now, attachedCount: 0, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketHot, wantMin: 7200, wantMax: 7200, wantAttached: 0},
		{name: "warm detached", state: State{Score: 2400, UpdatedAt: now, LastSeenAt: now}, now: now, attachedCount: 0, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketWarm, wantMin: 2400, wantMax: 2400, wantAttached: 0},
		{name: "cool detached", state: State{Score: 500, UpdatedAt: now, LastSeenAt: now}, now: now, attachedCount: 0, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketCool, wantMin: 500, wantMax: 500, wantAttached: 0},
		{name: "stale detached", state: State{Score: 7200, UpdatedAt: now, LastSeenAt: now.Add(-25 * time.Hour)}, now: now, attachedCount: 0, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketStale, wantMin: 7200, wantMax: 7200, wantAttached: 0},
		{name: "decays by half life", state: State{Score: 7200, UpdatedAt: now.Add(-8 * time.Hour), LastSeenAt: now}, now: now, attachedCount: 0, halfLife: 8 * time.Hour, staleAfter: 24 * time.Hour, wantBucket: BucketWarm, wantMin: 3599, wantMax: 3601, wantAttached: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bucket := Advance(tt.state, tt.now, tt.attachedCount, tt.halfLife, tt.staleAfter)
			if bucket != tt.wantBucket {
				t.Fatalf("bucket = %q, want %q", bucket, tt.wantBucket)
			}
			if got.Score < tt.wantMin || got.Score > tt.wantMax {
				t.Fatalf("score = %f, want between %f and %f", got.Score, tt.wantMin, tt.wantMax)
			}
			if got.AttachedCount != tt.wantAttached {
				t.Fatalf("attached count = %d, want %d", got.AttachedCount, tt.wantAttached)
			}
		})
	}
}
