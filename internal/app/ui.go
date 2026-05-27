package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

var newTmuxClient = func() tmuxcli.Client {
	return tmuxcli.Client{Process: process.Runner{}}
}

func effectiveUIClient(ctx context.Context, flags map[string]string) string {
	if client := strings.TrimSpace(flags["client"]); client != "" {
		return client
	}
	state, err := persistedSidebarState(ctx)
	if err != nil || !state.Open {
		return ""
	}
	return strings.TrimSpace(state.OwnerClient)
}

func loadSessionItems(ctx context.Context) ([]uity.SessionItem, error) {
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
	cfg := loadSidebarConfig(ctx)
	heatStates := decodePersistedHeat(persisted.Heat)
	attentionStates := attention.DecodeStateMap(persisted.AgentAttention)
	now := time.Now().UTC()
	current = strings.TrimSpace(current)
	names := sessions.ApplyOrder(sessionNames(sessions.FilterVisible(views, true)), persisted.SessionOrder)
	items := make([]uity.SessionItem, 0, len(names))
	viewsByName := make(map[string]sessions.View, len(views))
	for _, view := range views {
		viewsByName[view.Name] = view
	}
	slot := 1
	for _, name := range names {
		item := uity.SessionItem{Name: name, Current: name == current}
		if state, ok := heatStates[name]; ok && cfg.HeatColorsEnabled {
			item.Heat = string(sessionHeatBucket(state, now, cfg))
		}
		if cfg.AgentAttentionEnabled {
			if state, ok := attentionStateForSession(attentionStates, viewsByName[name]); ok {
				item.Attention = state.Attention
			}
		}
		if !sessions.IsNumericName(name) {
			item.Slot = slot
			slot++
		}
		items = append(items, item)
	}
	return items, nil
}

func loadProjectItems(ctx context.Context) []uity.ProjectItem {
	candidates, err := projectCandidates(ctx)
	if err != nil {
		return []uity.ProjectItem{}
	}
	items := make([]uity.ProjectItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, uity.ProjectItem{Name: filepath.Base(candidate.Path), Path: candidate.Path})
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

func sessionNames(views []sessions.View) []string {
	names := make([]string, 0, len(views))
	for _, view := range views {
		names = append(names, view.Name)
	}
	return names
}

func loadSidebarConfig(ctx context.Context) ports.ConfigSnapshot {
	cfg, err := newTmuxClient().LoadConfig(ctx)
	if err == nil && cfg.Loaded {
		return cfg
	}
	return defaultSidebarConfig()
}

func defaultSidebarConfig() ports.ConfigSnapshot {
	return ports.ConfigSnapshot{HeatColorsEnabled: true, HeatHalfLifeHours: 8, HeatStaleHours: 24, HeatRefreshSeconds: 5, HeatRecentHours: 1, ActivityDebugLog: false, AgentAttentionEnabled: true, AutoSortRecentEnabled: false}
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

func sessionHeatBucket(state heat.State, now time.Time, cfg ports.ConfigSnapshot) heat.Bucket {
	if recentSessionActivity(state, now, recentHeatWindow(cfg)) {
		return heat.BucketCurrent
	}
	return heat.BucketStale
}

func recentHeatWindow(cfg ports.ConfigSnapshot) time.Duration {
	if cfg.HeatRecentHours > 0 {
		return time.Duration(cfg.HeatRecentHours) * time.Hour
	}
	return time.Hour
}

func recentSessionActivity(state heat.State, now time.Time, window time.Duration) bool {
	return recentHeatTimestamp(state.LastVisitedAt, now, window) || recentHeatTimestamp(state.LastActiveAt, now, window)
}

func recentHeatTimestamp(timestamp time.Time, now time.Time, window time.Duration) bool {
	if timestamp.IsZero() {
		return false
	}
	age := now.Sub(timestamp)
	if age < 0 {
		return false
	}
	return age <= window
}
