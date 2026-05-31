package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	owner := strings.TrimSpace(state.OwnerClient)
	if owner == "" || !tmuxClientExists(ctx, owner) {
		if client := clientViewingSidebarPane(ctx); client != "" {
			return client
		}
	}
	return owner
}

func tmuxClientExists(ctx context.Context, client string) bool {
	out, err := tmux(ctx, "list-clients", "-F", "#{client_name}")
	if err != nil {
		return true
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == client {
			return true
		}
	}
	return false
}

func clientViewingSidebarPane(ctx context.Context) string {
	pane := strings.TrimSpace(os.Getenv("TMUX_PANE"))
	if pane == "" {
		return ""
	}
	windowID, err := tmux(ctx, "display-message", "-p", "-t", pane, "#{window_id}")
	if err != nil {
		return ""
	}
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	out, err := tmux(ctx, "list-clients", "-F", "#{client_name}\t#{window_id}")
	if err != nil {
		return ""
	}
	fallback := ""
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		client := strings.TrimSpace(fields[0])
		if fallback == "" {
			fallback = client
		}
		if strings.TrimSpace(fields[1]) == windowID {
			return client
		}
	}
	return fallback
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
	pinned := pinnedSessionSet(persisted.PinnedSessions)
	items := make([]uity.SessionItem, 0, len(names))
	viewsByName := make(map[string]sessions.View, len(views))
	for _, view := range views {
		viewsByName[view.Name] = view
	}
	heatDisplays := heat.DisplayByRecentActivity(names, heatStates, now, recentHeatWindow(cfg), cfg.HeatMaxHighlighted)
	slot := 1
	for _, name := range names {
		_, isPinned := pinned[name]
		item := uity.SessionItem{Name: name, Current: name == current, Pinned: isPinned, PinColor: persisted.PinColors[name]}
		if display, ok := heatDisplays[name]; ok && cfg.HeatColorsEnabled {
			item.Heat = string(display.Bucket)
			item.HeatIntensity = display.Intensity
		}
		if cfg.AgentAttentionEnabled {
			if state, ok := attentionStateForSession(attentionStates, viewsByName[name]); ok {
				item.Attention = state.Attention
			}
		}
		if cfg.MetadataSublineEnabled {
			if metadata, ok := persisted.Metadata[name]; ok {
				item.Metadata = gitStatusMetadataSubline(metadata)
			} else if path, ok := sessionMetadataPath(persisted.Sessions[name]); ok {
				item.Metadata = uity.SessionMetadataSubline{Kind: uity.MetadataKindDirectory, SessionName: name, Path: path}
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

func gitStatusMetadataSubline(status ports.GitStatus) uity.SessionMetadataSubline {
	return uity.SessionMetadataSubline{
		Kind:      uity.MetadataKindGit,
		Branch:    status.Branch,
		Clean:     status.Clean,
		Ahead:     status.Ahead,
		Behind:    status.Behind,
		Staged:    status.Staged,
		Modified:  status.Modified,
		Deleted:   status.Deleted,
		Renamed:   status.Renamed,
		Untracked: status.Untracked,
		Conflicts:       status.Conflicts,
		UpstreamMissing: !status.UpstreamConfigured,
	}
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
	cfg, err := newTmuxClient().LoadConfig(ctx)
	if err == nil && cfg.Loaded {
		return cfg
	}
	return defaultSidebarConfig()
}

func defaultSidebarConfig() ports.ConfigSnapshot {
	return ports.ConfigSnapshot{
		HeatColorsEnabled:      true,
		HeatHalfLifeHours:      8,
		HeatStaleHours:         24,
		HeatRefreshSeconds:     60,
		HeatRecentInterval:     time.Hour,
		HeatMaxHighlighted:     0,
		ActivityDebugLog:       false,
		AgentAttentionEnabled:  true,
		AutoSortRecentInterval: 0,
		RestoreSessionsMode:    "auto",
		ContinuumGraceSeconds:  3,
		MetadataSublineEnabled: true,
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
