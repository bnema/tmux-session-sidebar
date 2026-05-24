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
		agentCompleted     bool
		visited            bool
		halfLife           time.Duration
		staleAfter         time.Duration
		wantBucket         Bucket
		wantAttention      bool
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
			name: "agent completion latches attention",
			state: State{
				Score:            600,
				UpdatedAt:        now,
				LastActiveAt:     now,
				RecentActivityAt: now,
			},
			now:                now.Add(3 * time.Minute),
			agentCompleted:     true,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			wantBucket:         BucketCool,
			wantAttention:      true,
			wantLastActiveAt:   now,
			wantRecentActiveAt: now,
			wantScoreMin:       597,
			wantScoreMax:       598,
			wantTraceStatus:    TraceStatusAttentionStarted,
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
			wantBucket:         BucketCool,
			wantVisitedAt:      now,
			wantLastActiveAt:   now.Add(-3 * time.Minute),
			wantRecentActiveAt: now.Add(-3 * time.Minute),
			wantScoreMin:       599,
			wantScoreMax:       601,
			wantTraceStatus:    TraceStatusAttentionClearedOnVisit,
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
		{
			name: "stale expires attention",
			state: State{
				Score:            7200,
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-25 * time.Hour),
				RecentActivityAt: now.Add(-25 * time.Hour),
				Attention:        true,
			},
			now:                now,
			halfLife:           8 * time.Hour,
			staleAfter:         24 * time.Hour,
			wantBucket:         BucketStale,
			wantAttention:      false,
			wantLastActiveAt:   now.Add(-25 * time.Hour),
			wantRecentActiveAt: now.Add(-25 * time.Hour),
			wantScoreMin:       7200,
			wantScoreMax:       7200,
			wantTraceStatus:    TraceStatusAttentionExpiredAsStale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, trace := Advance(tt.state, tt.now, tt.observedActivity, tt.agentCompleted, tt.visited, tt.halfLife, tt.staleAfter)
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
			if trace.Status != tt.wantTraceStatus {
				t.Fatalf("trace status = %q, want %q", trace.Status, tt.wantTraceStatus)
			}
		})
	}
}

func TestAdvanceDoesNotRelatchCompletionForVisitedSession(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	state := State{Attention: true, UpdatedAt: now, LastActiveAt: now}

	got, trace := Advance(state, now.Add(time.Minute), false, true, true, 8*time.Hour, 24*time.Hour)

	if got.Attention {
		t.Fatal("attention = true, want false because visited sessions clear the bell even if completion is observed")
	}
	if trace.Status != TraceStatusAttentionClearedOnVisit {
		t.Fatalf("trace status = %q, want %q", trace.Status, TraceStatusAttentionClearedOnVisit)
	}
}

func TestAdvanceCompletionLatchesEvenForStaleSession(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	state := State{UpdatedAt: now, LastActiveAt: now.Add(-25 * time.Hour)}

	got, trace := Advance(state, now, false, true, false, 8*time.Hour, 24*time.Hour)

	if !got.Attention {
		t.Fatal("attention = false, want true when a stale long-running agent completes")
	}
	if trace.Status != TraceStatusAttentionStarted {
		t.Fatalf("trace status = %q, want %q", trace.Status, TraceStatusAttentionStarted)
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

	got, _ := Advance(state, now.Add(30*time.Second), true, false, false, 8*time.Hour, 24*time.Hour)
	if !got.Attention {
		t.Fatal("attention = false, want true until the session is visited")
	}
	if !got.LastVisitedAt.IsZero() {
		t.Fatalf("last visited at = %v, want zero", got.LastVisitedAt)
	}
}
