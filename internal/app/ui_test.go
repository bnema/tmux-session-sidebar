package app

import (
	"context"
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestEffectiveUIClientFallsBackToPersistedSidebarOwner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
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

func TestEffectiveUIClientFallsBackToClientViewingSidebarPaneWhenOwnerIsStale(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%sidebar")
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-clients) printf '/dev/current\t@1\n/dev/other\t@2\n' ;;
  display-message) printf '@1\n' ;;
esac
`)
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "/dev/stale"}
	}); err != nil {
		t.Fatalf("updateSidebarState error: %v", err)
	}

	if got := effectiveUIClient(ctx, map[string]string{}); got != "/dev/current" {
		t.Fatalf("effectiveUIClient with stale owner = %q, want client viewing sidebar pane", got)
	}
}

func TestLoadSessionItemsHydratesPinnedSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
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
      @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'off\n' ;;
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
	state.PinnedSessions = []string{"beta"}
	if err := store.Save(context.Background(), "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSessionItems(context.Background())
	if err != nil {
		t.Fatalf("loadSessionItems error: %v", err)
	}
	for _, item := range items {
		if item.Name == "beta" && !item.Pinned {
			t.Fatalf("beta Pinned = false, want true in %#v", items)
		}
		if item.Name == "alpha" && item.Pinned {
			t.Fatalf("alpha Pinned = true, want false in %#v", items)
		}
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
      @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
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
