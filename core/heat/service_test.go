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
		quietAfter         time.Duration
		wantBucket         Bucket
		wantAttention      bool
		wantVisitedAt      time.Time
		wantLastActiveAt   time.Time
		wantRecentActiveAt time.Time
		wantScoreMin       float64
		wantScoreMax       float64
	}{
		{
			name:               "activity adds heat and marks current",
			state:              State{},
			now:                now,
			observedActivity:   true,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			quietAfter:         2 * time.Minute,
			wantBucket:         BucketCurrent,
			wantLastActiveAt:   now,
			wantRecentActiveAt: now,
			wantScoreMin:       599,
			wantScoreMax:       601,
		},
		{
			name: "quiet latches attention after unseen recent activity",
			state: State{
				Score:            600,
				UpdatedAt:        now,
				LastActiveAt:     now,
				RecentActivityAt: now,
			},
			now:                now.Add(3 * time.Minute),
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			quietAfter:         2 * time.Minute,
			wantBucket:         BucketCool,
			wantAttention:      true,
			wantLastActiveAt:   now,
			wantRecentActiveAt: now,
			wantScoreMin:       597,
			wantScoreMax:       598,
		},
		{
			name: "visit clears latched attention",
			state: State{
				Score:            600,
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-3 * time.Minute),
				RecentActivityAt: now.Add(-3 * time.Minute),
				Attention:        true,
			},
			now:                now,
			visited:            true,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			quietAfter:         2 * time.Minute,
			wantBucket:         BucketCool,
			wantVisitedAt:      now,
			wantLastActiveAt:   now.Add(-3 * time.Minute),
			wantRecentActiveAt: now.Add(-3 * time.Minute),
			wantScoreMin:       599,
			wantScoreMax:       601,
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
			quietAfter:         2 * time.Minute,
			wantBucket:         BucketStale,
			wantLastActiveAt:   now.Add(-25 * time.Hour),
			wantRecentActiveAt: now.Add(-25 * time.Hour),
			wantScoreMin:       7200,
			wantScoreMax:       7200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, trace := Advance(tt.state, tt.now, tt.observedActivity, tt.visited, tt.halfLife, tt.staleAfter, tt.quietAfter)
			if trace.Bucket != tt.wantBucket {
				t.Fatalf("bucket = %q, want %q", trace.Bucket, tt.wantBucket)
			}
			if got.Attention != tt.wantAttention {
				t.Fatalf("attention = %v, want %v", got.Attention, tt.wantAttention)
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
		})
	}
}

func TestAdvanceHeatKeepsAttentionLatchedUntilVisit(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	state := State{
		Score:            600,
		UpdatedAt:        now,
		LastActiveAt:     now.Add(-3 * time.Minute),
		RecentActivityAt: now.Add(-3 * time.Minute),
		Attention:        true,
	}

	got, _ := Advance(state, now.Add(30*time.Second), true, false, 8*time.Hour, 24*time.Hour, 2*time.Minute)
	if !got.Attention {
		t.Fatal("attention = false, want true until the session is visited")
	}
	if !got.LastVisitedAt.IsZero() {
		t.Fatalf("last visited at = %v, want zero", got.LastVisitedAt)
	}
}
