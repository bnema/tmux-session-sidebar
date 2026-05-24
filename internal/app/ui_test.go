package app

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSessionHeatBucketUsesRecentSessionSwitchSignal(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	cfg := ports.ConfigSnapshot{HeatHalfLifeHours: 8, HeatStaleHours: 24, HeatRefreshSeconds: 5}

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

func TestLoadSessionItemsUsesDedicatedAgentAttentionState(t *testing.T) {
	for _, enabled := range []string{"on", "off"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("AGENT_ATTENTION", enabled)
			installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf '%s\n' "$AGENT_ATTENTION" ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\n' ;;
  list-sessions) printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n' ;;
  list-clients) ;;
esac
`)
			store := sessionOrderStore()
			state, err := store.Load(context.Background(), "tmux")
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			state.AgentAttention = attention.EncodeStateMap(map[string]attention.State{
				"$1": {Attention: true},
				"$2": {Attention: true},
			})
			if err := store.Save(context.Background(), "tmux", state); err != nil {
				t.Fatalf("save state: %v", err)
			}

			items, err := loadSessionItems(context.Background())
			if err != nil {
				t.Fatalf("loadSessionItems error: %v", err)
			}
			var alphaAttention, betaAttention bool
			var foundAlpha, foundBeta bool
			for _, item := range items {
				if item.Name == "alpha" {
					foundAlpha = true
					alphaAttention = item.Attention
				}
				if item.Name == "beta" {
					foundBeta = true
					betaAttention = item.Attention
				}
			}
			if !foundAlpha || !foundBeta {
				t.Fatalf("missing expected items: foundAlpha=%v foundBeta=%v items=%#v", foundAlpha, foundBeta, items)
			}
			if enabled == "on" && (!alphaAttention || !betaAttention) {
				t.Fatalf("attention = alpha:%v beta:%v, want both true when feature enabled", alphaAttention, betaAttention)
			}
			if enabled == "off" && (alphaAttention || betaAttention) {
				t.Fatalf("attention = alpha:%v beta:%v, want both false when feature disabled", alphaAttention, betaAttention)
			}
		})
	}
}
