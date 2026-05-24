package heat

import (
	"testing"
	"time"
)

func TestAdvanceHeat(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name               string
		state              State
		now                time.Time
		observedActivity   bool
		visited            bool
		halfLife           time.Duration
		staleAfter         time.Duration
		wantBucket         Bucket
		wantVisitedAt      time.Time
		wantLastActiveAt   time.Time
		wantRecentActiveAt time.Time
		wantScoreMin       float64
		wantScoreMax       float64
		wantTraceStatus    TraceStatus
	}{
		{
			name:               "activity adds heat and marks current",
			state:              State{},
			now:                now,
			observedActivity:   true,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			wantBucket:         BucketCurrent,
			wantLastActiveAt:   now,
			wantRecentActiveAt: now,
			wantScoreMin:       599,
			wantScoreMax:       601,
			wantTraceStatus:    TraceStatusActivityDetected,
		},
		{
			name: "visit only updates last visited",
			state: State{
				Score:            600,
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-3 * time.Minute),
				RecentActivityAt: now.Add(-3 * time.Minute),
			},
			now:                now,
			visited:            true,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			wantBucket:         BucketCool,
			wantVisitedAt:      now,
			wantLastActiveAt:   now.Add(-3 * time.Minute),
			wantRecentActiveAt: now.Add(-3 * time.Minute),
			wantScoreMin:       599,
			wantScoreMax:       601,
			wantTraceStatus:    TraceStatusNoChange,
		},
		{
			name: "stale wins after long inactivity",
			state: State{
				Score:            7200,
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-25 * time.Hour),
				RecentActivityAt: now.Add(-25 * time.Hour),
			},
			now:                now,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			wantBucket:         BucketStale,
			wantLastActiveAt:   now.Add(-25 * time.Hour),
			wantRecentActiveAt: now.Add(-25 * time.Hour),
			wantScoreMin:       7200,
			wantScoreMax:       7200,
			wantTraceStatus:    TraceStatusNoChange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, trace := Advance(tt.state, tt.now, tt.observedActivity, tt.visited, tt.halfLife, tt.staleAfter)
			if trace.Bucket != tt.wantBucket {
				t.Fatalf("bucket = %q, want %q", trace.Bucket, tt.wantBucket)
			}
			if !got.LastVisitedAt.Equal(tt.wantVisitedAt) {
				t.Fatalf("last visited at = %v, want %v", got.LastVisitedAt, tt.wantVisitedAt)
			}
			if !got.LastActiveAt.Equal(tt.wantLastActiveAt) {
				t.Fatalf("last active at = %v, want %v", got.LastActiveAt, tt.wantLastActiveAt)
			}
			if !got.RecentActivityAt.Equal(tt.wantRecentActiveAt) {
				t.Fatalf("recent activity at = %v, want %v", got.RecentActivityAt, tt.wantRecentActiveAt)
			}
			if got.Score < tt.wantScoreMin || got.Score > tt.wantScoreMax {
				t.Fatalf("score = %f, want between %f and %f", got.Score, tt.wantScoreMin, tt.wantScoreMax)
			}
			if trace.Status != tt.wantTraceStatus {
				t.Fatalf("trace status = %q, want %q", trace.Status, tt.wantTraceStatus)
			}
		})
	}
}

func TestAdvanceHeatKeepsVisitSignal(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	state := State{UpdatedAt: now, LastActiveAt: now}

	got, _ := Advance(state, now.Add(time.Minute), false, true, 8*time.Hour, 24*time.Hour)
	if !got.LastVisitedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("last visited at = %v, want %v", got.LastVisitedAt, now.Add(time.Minute))
	}
}
