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
	state.Sessions = reconcileLiveSessionMetadata(ctx, s.tmuxQuery, live, state.Sessions)
	state.SessionOrder = reconcileSessionOrder(state.SessionOrder, live)
	return s.store.Save(ctx, serverID, state)
}

func reconcileLiveSessionMetadata(ctx context.Context, query ports.TmuxQueryPort, live []ports.TmuxSessionSnapshot, current map[string]ports.SessionMetadata) map[string]ports.SessionMetadata {
	next := make(map[string]ports.SessionMetadata, len(live))
	for _, session := range live {
		if !sessions.IsPersistableName(session.Name) {
			continue
		}
		metadata, hadMetadata := current[session.Name]
		path, err := query.SessionPath(ctx, session.Name)
		if err != nil || !usefulPath(path) {
			if hadMetadata {
				next[session.Name] = metadata
			}
			continue
		}
		if strings.TrimSpace(metadata.Kind) == "" {
			metadata.Kind = "captured"
		}
		metadata.LastPath = strings.TrimSpace(path)
		next[session.Name] = metadata
	}
	return next
}

func reconcileSessionOrder(order []string, live []ports.TmuxSessionSnapshot) []string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ApplyOrder(liveNames, order)
}

func orderedPersistedSessionNames(state ports.PersistedState) []string {
	names := make([]string, 0, len(state.Sessions))
	for name := range state.Sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	return sessions.ApplyOrder(names, state.SessionOrder)
}

func isRestorableSessionName(name string) bool {
	return sessions.IsPersistableName(name)
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
