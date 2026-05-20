package runtime

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type RestoreReport struct {
	Restored       []string
	Skipped        []string
	Failed         map[string]error
	SystemFailures map[string]error
}

func (s *Service) RestorePersistedSessions(ctx context.Context, serverID string, home string) RestoreReport {
	report := RestoreReport{Failed: map[string]error{}, SystemFailures: map[string]error{}}
	if s.store == nil {
		report.SystemFailures["store"] = ErrMissingStateStore
		return report
	}
	if s.tmuxQuery == nil {
		report.SystemFailures["query"] = ErrMissingTmuxQuery
		return report
	}
	if s.tmuxCtl == nil {
		report.SystemFailures["control"] = ErrMissingTmuxControl
		return report
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		report.SystemFailures["load"] = err
		return report
	}
	live, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		report.SystemFailures["list"] = err
		return report
	}

	liveNames := map[string]bool{}
	for _, session := range live {
		liveNames[session.Name] = true
	}
	for _, name := range orderedPersistedSessionNames(state) {
		metadata := state.Sessions[name]
		if !isRestorableSessionName(name) || liveNames[name] {
			report.Skipped = append(report.Skipped, name)
			continue
		}
		path := restorePath(metadata, home)
		if err := s.tmuxCtl.CreateSession(ctx, name, path); err != nil {
			report.Failed[name] = err
			continue
		}
		report.Restored = append(report.Restored, name)
	}
	return report
}

func (s *Service) CaptureLiveSessions(ctx context.Context, serverID string) error {
	if s.store == nil {
		return ErrMissingStateStore
	}
	if s.tmuxQuery == nil {
		return ErrMissingTmuxQuery
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return err
	}
	live, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		return err
	}
	if state.Sessions == nil {
		state.Sessions = map[string]ports.SessionMetadata{}
	}
	for _, session := range live {
		if !isRestorableSessionName(session.Name) {
			continue
		}
		path, err := s.tmuxQuery.SessionPath(ctx, session.Name)
		if err != nil || !usefulPath(path) {
			continue
		}
		metadata := state.Sessions[session.Name]
		if strings.TrimSpace(metadata.Kind) == "" {
			metadata.Kind = "captured"
		}
		metadata.LastPath = strings.TrimSpace(path)
		state.Sessions[session.Name] = metadata
	}
	return s.store.Save(ctx, serverID, state)
}

func orderedPersistedSessionNames(state ports.PersistedState) []string {
	used := map[string]bool{}
	ordered := make([]string, 0, len(state.Sessions))
	for _, name := range state.SessionOrder {
		if _, ok := state.Sessions[name]; ok && !used[name] {
			ordered = append(ordered, name)
			used[name] = true
		}
	}
	remaining := make([]string, 0, len(state.Sessions)-len(ordered))
	for name := range state.Sessions {
		if !used[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	return append(ordered, remaining...)
}

func isRestorableSessionName(name string) bool {
	return name != "" && sessions.ValidateName(name) == nil && !sessions.IsNumericName(name) && !sessions.IsHiddenName(name)
}

func restorePath(metadata ports.SessionMetadata, home string) string {
	for _, candidate := range []string{metadata.LastPath, metadata.ProjectPath} {
		if usefulPath(candidate) {
			return strings.TrimSpace(candidate)
		}
	}
	if usefulPath(home) {
		return strings.TrimSpace(home)
	}
	return "."
}

func usefulPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
