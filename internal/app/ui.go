package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/attention"
	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/viewmodel"
)

func effectiveUIClient(ctx context.Context, flags map[string]string) string {
	return newSidebarOwnerResolver().ResolveActionClient(ctx, flags)
}

func loadSessionItems(ctx context.Context) ([]viewmodel.SessionItem, error) {
	return loadSessionItemsWithConfig(ctx, loadSidebarConfig(ctx))
}

func loadSessionItemsWithConfig(ctx context.Context, cfg ports.ConfigSnapshot) ([]viewmodel.SessionItem, error) {
	current, err := tmux(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("getting current tmux session: %w", err)
	}
	views, err := runtimeService().SessionViews(ctx)
	if err != nil {
		return nil, err
	}
	persisted, err := loadSidebarState(ctx)
	if err != nil {
		return nil, err
	}
	current = strings.TrimSpace(current)
	items, _ := sessionItemsFromState(current, views, persisted, cfg) // by-name index is only needed by tree layout rendering.
	slot := 1
	for i := range items {
		if !sessions.IsNumericName(items[i].Name) {
			items[i].Slot = slot
			slot++
		}
	}
	return items, nil
}

func gitStatusMetadataSubline(status ports.GitStatus) viewmodel.SessionMetadataSubline {
	return viewmodel.SessionMetadataSubline{
		Kind:            viewmodel.MetadataKindGit,
		Branch:          status.Branch,
		Clean:           status.Clean,
		Ahead:           status.Ahead,
		Behind:          status.Behind,
		UpstreamAhead:   status.UpstreamAhead,
		UpstreamBehind:  status.UpstreamBehind,
		Staged:          status.Staged,
		Modified:        status.Modified,
		Deleted:         status.Deleted,
		Renamed:         status.Renamed,
		Untracked:       status.Untracked,
		Conflicts:       status.Conflicts,
		UpstreamMissing: !status.UpstreamConfigured,
	}
}

func loadProjectItems(ctx context.Context) []viewmodel.ProjectItem {
	candidates, err := projectCandidates(ctx)
	if err != nil {
		return []viewmodel.ProjectItem{}
	}
	items := make([]viewmodel.ProjectItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, viewmodel.ProjectItem{Name: filepath.Base(candidate.Path), Path: candidate.Path})
	}
	return items
}

func attentionStateForSession(states map[string]attention.State, view sessions.View) (attention.State, bool) {
	if state, ok := states[view.SessionID]; ok {
		return state, true
	}
	state, ok := states[view.Name]
	return state, ok
}

func pinnedSessionSet(names []string) map[string]struct{} {
	pinned := make(map[string]struct{}, len(names))
	for _, name := range names {
		pinned[name] = struct{}{}
	}
	return pinned
}

func sessionNames(views []sessions.View) []string {
	names := make([]string, 0, len(views))
	for _, view := range views {
		names = append(names, view.Name)
	}
	return names
}

func loadSidebarConfig(ctx context.Context) ports.ConfigSnapshot {
	cfg, err := runtimeMultiplexer().LoadConfig(ctx)
	if err != nil || !cfg.Loaded {
		cfg = defaultSidebarConfig()
	}
	resolved := cfg
	if err := withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
		resolved = withResolvedColorSchemeAppearance(ctx, cfg, logger)
		return nil
	}); err != nil {
		return withResolvedColorSchemeAppearance(ctx, cfg, nil)
	}
	return resolved
}

func withResolvedColorSchemeAppearance(ctx context.Context, cfg ports.ConfigSnapshot, logger ports.LoggerPort) ports.ConfigSnapshot {
	mode := config.ParseColorSchemeMode(string(cfg.ColorSchemeMode))
	cfg.ColorSchemeMode = mode
	system := config.SystemColorSchemeNoPreference
	systemValue := any("skipped")
	var systemErr error
	if mode == config.ColorSchemeModeSystem {
		systemValue = system
		if source := runtimeSystemColorScheme(); source != nil {
			preference, err := source.CurrentPreference(ctx)
			if err == nil {
				system = preference
				systemValue = preference
			} else {
				systemErr = err
			}
		}
	}
	cfg.ColorSchemeAppearance = config.ResolveColorSchemeAppearance(mode, system)
	if logger != nil {
		fields := []ports.LogField{
			{Key: "mode", Value: cfg.ColorSchemeMode},
			{Key: "system", Value: systemValue},
			{Key: "appearance", Value: cfg.ColorSchemeAppearance},
		}
		if systemErr != nil {
			fields = append(fields, ports.LogField{Key: "system_error", Value: systemErr.Error()})
		}
		logger.Debug("color-scheme", fields)
	}
	return cfg
}

func defaultSidebarConfig() ports.ConfigSnapshot {
	return ports.ConfigSnapshot{
		HeatColorsEnabled:       true,
		HeatHalfLifeHours:       8,
		HeatStaleHours:          24,
		HeatRefreshSeconds:      60,
		HeatRecentInterval:      time.Hour,
		HeatMaxHighlighted:      0,
		ActivityDebugLog:        false,
		AgentAttentionEnabled:   true,
		AgentAttentionAnimation: config.AgentAttentionAnimationPulse,
		AutoSortRecentInterval:  0,
		RestoreSessionsMode:     "auto",
		ContinuumGraceSeconds:   3,
		ColorSchemeMode:         config.ColorSchemeModeSystem,
		ColorSchemeAppearance:   config.ColorSchemeAppearanceLight,
		MetadataSublineEnabled:  true,
		MetadataInactiveEnabled: true,
	}
}

func decodePersistedHeat(raw map[string][]byte) map[string]heat.State {
	decoded := make(map[string]heat.State, len(raw))
	for name, data := range raw {
		if len(data) == 0 {
			continue
		}
		var state heat.State
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		decoded[name] = state
	}
	return decoded
}

func recentHeatWindow(cfg ports.ConfigSnapshot) time.Duration {
	if cfg.HeatRecentInterval > 0 {
		return cfg.HeatRecentInterval
	}
	return time.Hour
}
