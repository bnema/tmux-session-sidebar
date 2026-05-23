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
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

var newTmuxClient = func() tmuxcli.Client {
	return tmuxcli.Client{Process: process.Runner{}}
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
	persisted, _ := loadSidebarState(ctx)
	cfg := loadSidebarConfig(ctx)
	heatStates := decodePersistedHeat(persisted.Heat)
	now := time.Now().UTC()
	current = strings.TrimSpace(current)
	names := sessions.ApplyOrder(sessionNames(sessions.FilterVisible(views, true)), persisted.SessionOrder)
	items := make([]uity.SessionItem, 0, len(names))
	slot := 1
	for _, name := range names {
		item := uity.SessionItem{Name: name, Current: name == current}
		if state, ok := heatStates[name]; ok {
			item.Attention = state.Attention && !item.Current
			if cfg.HeatColorsEnabled {
				item.Heat = string(sessionHeatBucket(state, now, cfg))
			}
		}
		if !sessions.IsNumericName(name) && slot <= 10 {
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
	return ports.ConfigSnapshot{HeatColorsEnabled: true, HeatHalfLifeHours: 8, HeatStaleHours: 24, HeatRefreshSeconds: 5, AttentionQuietSeconds: 120, ActivityDebugLog: false}
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
	halfLife := 8 * time.Hour
	if cfg.HeatHalfLifeHours > 0 {
		halfLife = time.Duration(cfg.HeatHalfLifeHours) * time.Hour
	}
	staleAfter := 24 * time.Hour
	if cfg.HeatStaleHours > 0 {
		staleAfter = time.Duration(cfg.HeatStaleHours) * time.Hour
	}
	refreshWindow := 6 * time.Second
	if cfg.HeatRefreshSeconds > 0 {
		// The +1s grace keeps the equality-based "current" state from flickering when
		// the next daemon poll lands slightly late.
		refreshWindow = time.Duration(cfg.HeatRefreshSeconds+1) * time.Second
	}
	// Equality is intentional here: a session is only "current" when activity was observed
	// in the same update cycle as the last state advance, and refreshWindow keeps that signal
	// visible until the next daemon poll.
	active := !state.UpdatedAt.IsZero() && !state.RecentActivityAt.IsZero() && state.RecentActivityAt.Equal(state.UpdatedAt) && now.Sub(state.UpdatedAt) <= refreshWindow
	return heat.BucketFor(state, now, active, halfLife, staleAfter)
}
