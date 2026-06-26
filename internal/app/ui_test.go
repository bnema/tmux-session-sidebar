package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/internal/core/attention"
	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestDefaultSidebarConfigIncludesSidebarWidth(t *testing.T) {
	cfg := defaultSidebarConfig()
	if cfg.Width != "30" {
		t.Fatalf("default width = %q, want 30", cfg.Width)
	}
}

func TestEffectiveUIClientFallsBackToPersistedSidebarOwner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  list-clients) printf 'client-1\nclient-2\n' ;;
esac
`)
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

func TestSidebarActionsResolveUIClientAtActionTime(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  switch-client) ;;
  list-clients) printf 'client-2\n' ;;
esac
`)
	ctx := context.Background()
	if err := saveSidebarVisibility(ctx, true, "client-1"); err != nil {
		t.Fatalf("save initial sidebar visibility: %v", err)
	}
	actions := buildSidebarActions(ctx, map[string]string{}, io.Discard, nil, nil)
	if err := saveSidebarVisibility(ctx, true, "client-2"); err != nil {
		t.Fatalf("save moved sidebar visibility: %v", err)
	}

	if ok := actions.SwitchSession("beta"); !ok {
		t.Fatal("SwitchSession = false, want true")
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	if got := string(logData); !strings.Contains(got, "switch-client -c client-2 -t =beta:") {
		t.Fatalf("switch target used stale client, log = %q", got)
	}
}

func TestEffectiveUIClientFallsBackToClientViewingSidebarPaneWhenOwnerIsStale(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%sidebar")
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-clients) printf '/dev/current\t@1\talpha\n/dev/other\t@2\tbeta\n' ;;
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
      @session-sidebar-width) printf '30\n' ;;
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
	state.PinColors = map[string]string{"beta": "#38bdf8"}
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
		if item.Name == "beta" && item.PinColor != "#38bdf8" {
			t.Fatalf("beta PinColor = %q, want #38bdf8 in %#v", item.PinColor, items)
		}
		if item.Name == "alpha" && item.Pinned {
			t.Fatalf("alpha Pinned = true, want false in %#v", items)
		}
	}
}

type fixedSystemColorSchemePort struct {
	preference config.SystemColorSchemePreference
	err        error
}

func (f fixedSystemColorSchemePort) CurrentPreference(context.Context) (config.SystemColorSchemePreference, error) {
	return f.preference, f.err
}

func (fixedSystemColorSchemePort) Watch(context.Context) (<-chan config.SystemColorSchemePreference, <-chan error, error) {
	return nil, nil, nil
}

func TestLoadSidebarConfigWritesColorSchemeResolutionToActivityLog(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '60\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'on\n' ;;
      @session-sidebar-agent-attention) printf 'off\n' ;;
      @session-sidebar-agent-attention-animation) printf 'pulse\n' ;;
      @session-sidebar-auto-sort-recent) printf 'off\n' ;;
      @session-sidebar-restore-sessions) printf 'auto\n' ;;
      @session-sidebar-continuum-grace-seconds) printf '3\n' ;;
      @session-sidebar-color-scheme) printf 'system\n' ;;
      @session-sidebar-metadata-subline) printf 'on\n' ;;
      @session-sidebar-metadata-inactive) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  *) printf 'unexpected tmux command: %s\n' "$*" >&2; exit 1 ;;
esac
`)
	deps := runtimeDependencies()
	updatedDeps := deps
	updatedDeps.SystemColorSchemePort = fixedSystemColorSchemePort{preference: config.SystemColorSchemePreferLight}
	SetRuntimeDependencies(updatedDeps)
	t.Cleanup(func() { SetRuntimeDependencies(deps) })

	cfg := loadSidebarConfig(context.Background())
	if cfg.ColorSchemeAppearance != config.ColorSchemeAppearanceLight {
		t.Fatalf("ColorSchemeAppearance = %q, want %q", cfg.ColorSchemeAppearance, config.ColorSchemeAppearanceLight)
	}

	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	log := string(content)
	if !strings.Contains(log, "debug: color-scheme") {
		t.Fatalf("activity log missing color-scheme trace: %q", log)
	}
	if !strings.Contains(log, "mode=system") {
		t.Fatalf("activity log missing mode: %q", log)
	}
	if !strings.Contains(log, "system=prefer-light") {
		t.Fatalf("activity log missing detected system preference: %q", log)
	}
	if !strings.Contains(log, "appearance=light") {
		t.Fatalf("activity log missing resolved appearance: %q", log)
	}
}

func TestLoadSessionItemsHydratesCachedMetadataWithoutFetchingGit(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
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
      @session-sidebar-metadata-subline) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\n' ;;
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  list-clients) ;;
  *) printf 'unexpected tmux command: %s\n' "$*" >&2; exit 1 ;;
esac
`)
	store := sessionOrderStore()
	state, err := store.Load(context.Background(), "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Metadata = map[string]ports.GitStatus{"alpha": {Branch: "main", Clean: true}}
	if err := store.Save(context.Background(), "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	items, err := loadSessionItems(context.Background())
	if err != nil {
		t.Fatalf("loadSessionItems error: %v", err)
	}
	if len(items) != 1 || items[0].Metadata.Kind != uity.MetadataKindGit || items[0].Metadata.Branch != "main" || !items[0].Metadata.Clean {
		t.Fatalf("items metadata = %#v", items)
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
    if [ "$2" = "-g" ] && [ $# -eq 2 ]; then
      printf '@session-sidebar-key M-b\n'
      printf '@session-sidebar-width 30\n'
      printf '@session-sidebar-project-roots \n'
      printf '@session-sidebar-close-after-switch off\n'
      printf '@session-sidebar-heat-colors on\n'
      printf '@session-sidebar-heat-half-life-hours 8\n'
      printf '@session-sidebar-heat-stale-hours 24\n'
      printf '@session-sidebar-heat-refresh-seconds 60\n'
      printf '@session-sidebar-heat-recent 1h\n'
      printf '@session-sidebar-heat-max-highlighted 0\n'
      printf '@session-sidebar-activity-debug-log off\n'
      printf '@session-sidebar-agent-attention %s\n' "$AGENT_ATTENTION"
      printf '@session-sidebar-agent-attention-animation pulse\n'
      printf '@session-sidebar-auto-sort-recent off\n'
      printf '@session-sidebar-restore-sessions auto\n'
      printf '@session-sidebar-continuum-grace-seconds 0\n'
      printf '@session-sidebar-metadata-subline off\n'
      printf '@session-sidebar-metadata-inactive off\n'
    else
      case "$3" in
        @session-sidebar-key) printf 'M-b\n' ;;
        @session-sidebar-width) printf '30\n' ;;
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
      esac
    fi ;;
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
