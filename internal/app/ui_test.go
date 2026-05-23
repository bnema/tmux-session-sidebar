package app

import (
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSessionHeatBucketUsesRecentSessionSwitchSignal(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	cfg := ports.ConfigSnapshot{HeatHalfLifeHours: 8, HeatStaleHours: 24, HeatRefreshSeconds: 5, AttentionQuietSeconds: 60}

	tests := []struct {
		name  string
		state heat.State
		want  heat.Bucket
	}{
		{
			name: "recent switch stays highlighted",
			state: heat.State{
				LastVisitedAt: now.Add(-5 * time.Second),
			},
			want: heat.BucketCurrent,
		},
		{
			name: "recent activity without switch signal stays gray",
			state: heat.State{
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-5 * time.Second),
				RecentActivityAt: now.Add(-5 * time.Second),
			},
			want: heat.BucketStale,
		},
		{
			name: "historical heat without switch signal stays gray",
			state: heat.State{
				Score:        50000,
				UpdatedAt:    now,
				LastActiveAt: now.Add(-10 * time.Minute),
			},
			want: heat.BucketStale,
		},
		{
			name: "old switch signal expires back to gray",
			state: heat.State{
				LastVisitedAt: now.Add(-45 * time.Second),
			},
			want: heat.BucketStale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionHeatBucket(tt.state, now, cfg); got != tt.want {
				t.Fatalf("sessionHeatBucket() = %q, want %q", got, tt.want)
			}
		})
	}
}
