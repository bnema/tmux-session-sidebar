package app

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestEffectiveUIClientFallsBackToPersistedSidebarOwner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-1"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}

	if got := effectiveUIClient(ctx, map[string]string{}); got != "client-1" {
		t.Fatalf("effectiveUIClient without flag = %q, want persisted owner", got)
	}
	if got := effectiveUIClient(ctx, map[string]string{"client": "client-2"}); got != "client-2" {
		t.Fatalf("effectiveUIClient with flag = %q, want explicit client", got)
	}
}

func TestSessionHeatBucketUsesRecentSessionSwitchSignal(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	cfg := ports.ConfigSnapshot{HeatHalfLifeHours: 8, HeatStaleHours: 24, HeatRefreshSeconds: 5, HeatRecentHours: 1}

	tests := []struct {
		name            string
		heatRecentHours int
		state           heat.State
		want            heat.Bucket
	}{
		{
			name: "recent switch stays highlighted",
			state: heat.State{
				LastVisitedAt: now.Add(-5 * time.Second),
			},
			want: heat.BucketCurrent,
		},
		{
			name: "recent terminal activity without switch signal stays highlighted",
			state: heat.State{
				UpdatedAt:        now,
				LastActiveAt:     now.Add(-5 * time.Second),
				RecentActivityAt: now.Add(-5 * time.Second),
			},
			want: heat.BucketCurrent,
		},
		{
			name: "historical heat older than an hour stays gray",
			state: heat.State{
				Score:        50000,
				UpdatedAt:    now,
				LastActiveAt: now.Add(-2 * time.Hour),
			},
			want: heat.BucketStale,
		},
		{
			name: "active within the last hour stays highlighted",
			state: heat.State{
				LastVisitedAt: now.Add(-59 * time.Minute),
			},
			want: heat.BucketCurrent,
		},
		{
			name: "switch signal older than an hour expires back to gray",
			state: heat.State{
				LastVisitedAt: now.Add(-61 * time.Minute),
			},
			want: heat.BucketStale,
		},
		{
			name:            "configured two hour window keeps older signal highlighted",
			heatRecentHours: 2,
			state: heat.State{
				LastVisitedAt: now.Add(-90 * time.Minute),
			},
			want: heat.BucketCurrent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCfg := cfg
			if tt.heatRecentHours > 0 {
				testCfg.HeatRecentHours = tt.heatRecentHours
			}
			if got := sessionHeatBucket(tt.state, now, testCfg); got != tt.want {
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
      @session-sidebar-heat-recent-hours) printf '1\n' ;;
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
