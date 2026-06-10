package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/persisted"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type RestoreReport struct {
	Restored       []string
	Skipped        []string
	Failed         map[string]error
	SystemFailures map[string]error
}

const (
	defaultHeatHalfLife   = 8 * time.Hour
	defaultHeatStaleAfter = 24 * time.Hour
	paneSampleTailLines   = 8
)

type paneObservation struct {
	SessionID   string
	SessionName string
	PaneID      string
	Fingerprint string
	Sampled     bool
}

// paneActivityQuery is an optional extension over ports.TmuxQueryPort that enables
// pane sampling. collectPaneObservations gracefully degrades to nil observations when
// the query does not implement it.
type paneActivityQuery interface {
	ListPanes(ctx context.Context) ([]ports.TmuxPaneSnapshot, error)
	CapturePaneText(ctx context.Context, paneID string, tailLines int) (string, error)
}

type heatConfig struct {
	halfLife   time.Duration
	staleAfter time.Duration
}

type heatCaptureResult struct {
	captured         bool
	complete         bool
	completeSessions map[string]bool
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
	_, err := s.captureLiveSessions(ctx, serverID, nil, false)
	return err
}

func (s *Service) CaptureLiveSessionsProtected(ctx context.Context, serverID string) (bool, error) {
	return s.captureLiveSessions(ctx, serverID, nil, true)
}

func (s *Service) CaptureSessionHeat(ctx context.Context, serverID string) error {
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
	_ = s.captureHeatIntoState(ctx, &state, live, s.loadHeatConfig(ctx))
	return s.store.Save(ctx, serverID, state)
}

func (s *Service) CaptureLiveSessionsWithConfig(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) error {
	_, err := s.captureLiveSessions(ctx, serverID, &snapshot, false)
	return err
}

func (s *Service) CaptureLiveSessionsWithConfigProtected(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) (bool, error) {
	return s.captureLiveSessions(ctx, serverID, &snapshot, true)
}

// captureLiveSessions is the shared implementation for all CaptureLiveSessions variants.
// snapshot controls heat config and recent-session-order behavior: non-nil activates
// captureHeatIntoState with the provided config and applyRecentSessionOrderAfterCapture;
// nil loads heat config from tmux and skips recent-session-order.
// protectEmpty controls whether a destructive-empty skip is applied: when true and no
// persistable sessions are live but the persisted state is meaningful, the capture is
// skipped to guard against overwriting restored state during startup/restore windows.
func (s *Service) captureLiveSessions(ctx context.Context, serverID string, snapshot *ports.ConfigSnapshot, protectEmpty bool) (bool, error) {
	if s.store == nil {
		return false, ErrMissingStateStore
	}
	if s.tmuxQuery == nil {
		return false, ErrMissingTmuxQuery
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return false, err
	}
	live, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	if protectEmpty && persistableSessionCount(live) == 0 && persisted.IsMeaningful(state) {
		return false, nil
	}
	state.Sessions = reconcileLiveSessionMetadata(ctx, s.tmuxQuery, live, state.Sessions)
	state.SessionOrder = reconcileSessionOrder(state.SessionOrder, live)
	state.PinnedSessions = reconcilePinnedSessions(state.PinnedSessions, live)
	state.PinColors = reconcilePinColors(state.PinColors, live)
	if snapshot != nil {
		capture := s.captureHeatIntoState(ctx, &state, live, heatConfigFromSnapshot(*snapshot))
		applyRecentSessionOrderAfterCapture(&state, live, *snapshot, time.Now(), capture)
	} else {
		_ = s.captureHeatIntoState(ctx, &state, live, s.loadHeatConfig(ctx))
	}
	return true, s.store.Save(ctx, serverID, state)
}

func (s *Service) CaptureSessionHeatWithConfig(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) error {
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
	capture := s.captureHeatIntoState(ctx, &state, live, heatConfigFromSnapshot(snapshot))
	applyRecentSessionOrderAfterCapture(&state, live, snapshot, time.Now(), capture)
	return s.store.Save(ctx, serverID, state)
}

func (s *Service) ResetTransientHeatState(ctx context.Context, serverID string) error {
	if s.store == nil {
		return ErrMissingStateStore
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return fmt.Errorf("reset transient heat state: load: %w", err)
	}
	decoded := decodeHeatStateMap(state.Heat)
	for name, heatState := range decoded {
		decoded[name] = clearTransientHeatState(heatState)
	}
	state.Heat = encodeHeatStateMap(decoded)
	if err := s.store.Save(ctx, serverID, state); err != nil {
		return fmt.Errorf("reset transient heat state: save: %w", err)
	}
	return nil
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

func reconcilePinnedSessions(pinned []string, live []ports.TmuxSessionSnapshot) []string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ReconcilePinned(pinned, liveNames)
}

func reconcilePinColors(colors map[string]string, live []ports.TmuxSessionSnapshot) map[string]string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ReconcileNamedStrings(colors, liveNames)
}

// persistableSessionCount returns the number of live sessions whose name
// qualifies for persistence (valid, non-hidden, non-numeric).
func persistableSessionCount(live []ports.TmuxSessionSnapshot) int {
	count := 0
	for _, s := range live {
		if sessions.IsPersistableName(s.Name) {
			count++
		}
	}
	return count
}

func orderedPersistedSessionNames(state ports.PersistedState) []string {
	names := make([]string, 0, len(state.Sessions))
	for name := range state.Sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	return sessions.ApplyOrder(names, state.SessionOrder)
}

func applyRecentSessionOrder(state *ports.PersistedState, live []ports.TmuxSessionSnapshot, cfg ports.ConfigSnapshot, now time.Time) {
	applyRecentSessionOrderAfterCapture(state, live, cfg, now, heatCaptureResult{captured: true, complete: true})
}

func applyRecentSessionOrderAfterCapture(state *ports.PersistedState, live []ports.TmuxSessionSnapshot, cfg ports.ConfigSnapshot, now time.Time, capture heatCaptureResult) {
	if state == nil || cfg.AutoSortRecentInterval <= 0 || !capture.captured {
		return
	}
	if state.Sidebar != nil {
		lastRunAt, ok := autoSortRecentLastRunAt(*state.Sidebar)
		if ok && now.Sub(lastRunAt) < cfg.AutoSortRecentInterval {
			return
		}
	}
	ordered := orderSessionsByRecentActivityPinned(state.SessionOrder, live, decodeHeatStateMap(state.Heat), state.PinnedSessions)
	if !capture.complete {
		reorderCompleteSidebarLayoutCategories(state.SidebarLayout, ordered, state.PinnedSessions, capture.completeSessions)
		return
	}
	state.SessionOrder = ordered
	reorderSidebarLayoutCategories(state.SidebarLayout, state.SessionOrder, state.PinnedSessions)
	if state.Sidebar == nil {
		state.Sidebar = &ports.SidebarState{}
	}
	state.Sidebar.AutoSortRecentRunAt = now.Format(time.RFC3339Nano)
	state.Sidebar.AutoSortRecentRunDate = ""
}

func reorderSidebarLayoutCategories(layout *ports.SidebarLayout, order []string, pinned []string) {
	reorderSidebarLayoutCategoriesWhere(layout, order, pinned, nil)
}

func reorderCompleteSidebarLayoutCategories(layout *ports.SidebarLayout, order []string, pinned []string, completeSessions map[string]bool) {
	reorderSidebarLayoutCategoriesWhere(layout, order, pinned, func(refs []ports.SidebarLayoutSessionRef) bool {
		for _, ref := range refs {
			complete, ok := completeSessions[ref.Name]
			if ok && !complete {
				return false
			}
		}
		return true
	})
}

func reorderSidebarLayoutCategoriesWhere(layout *ports.SidebarLayout, order []string, pinned []string, shouldReorder func([]ports.SidebarLayoutSessionRef) bool) {
	if layout == nil {
		return
	}
	orderIndex := make(map[string]int, len(order))
	for i, name := range order {
		orderIndex[name] = i
	}
	for itemIndex := range layout.Items {
		category := layout.Items[itemIndex].Category
		if category == nil || shouldReorder != nil && !shouldReorder(category.Sessions) {
			continue
		}
		category.Sessions = reorderSidebarLayoutCategorySessions(category.Sessions, orderIndex, pinned)
	}
}

func reorderSidebarLayoutCategorySessions(refs []ports.SidebarLayoutSessionRef, orderIndex map[string]int, pinned []string) []ports.SidebarLayoutSessionRef {
	if len(refs) == 0 {
		return refs
	}
	byName := make(map[string]ports.SidebarLayoutSessionRef, len(refs))
	anchor := make([]string, 0, len(refs))
	ordered := append([]ports.SidebarLayoutSessionRef(nil), refs...)
	for _, ref := range refs {
		byName[ref.Name] = ref
		anchor = append(anchor, ref.Name)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftOK := orderIndex[ordered[i].Name]
		right, rightOK := orderIndex[ordered[j].Name]
		switch {
		case leftOK && rightOK:
			return left < right
		case leftOK:
			return true
		case rightOK:
			return false
		default:
			return false
		}
	})
	orderedNames := make([]string, 0, len(ordered))
	for _, ref := range ordered {
		orderedNames = append(orderedNames, ref.Name)
	}
	pinnedNames := sessions.ApplyPinnedPositions(anchor, orderedNames, pinned)
	result := make([]ports.SidebarLayoutSessionRef, 0, len(refs))
	for _, name := range pinnedNames {
		result = append(result, byName[name])
	}
	return result
}

func autoSortRecentLastRunAt(sidebar ports.SidebarState) (time.Time, bool) {
	if runAt, err := time.Parse(time.RFC3339Nano, sidebar.AutoSortRecentRunAt); err == nil {
		return runAt, true
	}
	if runDate, err := time.ParseInLocation("2006-01-02", sidebar.AutoSortRecentRunDate, time.Local); err == nil {
		return runDate, true
	}
	return time.Time{}, false
}

func orderSessionsByRecentActivityPinned(order []string, live []ports.TmuxSessionSnapshot, heatStates map[string]heat.State, pinned []string) []string {
	ordered := reconcileSessionOrder(order, live)
	anchor := append([]string(nil), ordered...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := heatStates[ordered[i]].LastActiveAt
		right := heatStates[ordered[j]].LastActiveAt
		switch {
		case left.IsZero() && right.IsZero():
			return false
		case left.IsZero():
			return false
		case right.IsZero():
			return true
		default:
			return left.After(right)
		}
	})
	return sessions.ApplyPinnedPositions(anchor, ordered, pinned)
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

func (s *Service) captureHeatIntoState(ctx context.Context, state *ports.PersistedState, live []ports.TmuxSessionSnapshot, cfg heatConfig) heatCaptureResult {
	if state == nil {
		return heatCaptureResult{}
	}
	clients, clientsErr := s.tmuxQuery.ListClients(ctx)
	observations, observationsErr := collectPaneObservations(ctx, s.tmuxQuery)
	// Heat collection is best-effort: if ListClients or collectPaneObservations fails,
	// return early so only reconcileLiveSessionHeat, state.Heat updates, and trace logging
	// are skipped. Session metadata (Sessions, SessionOrder) is still persisted by callers.
	if clientsErr != nil || observationsErr != nil {
		return heatCaptureResult{}
	}
	nextHeat, traces := reconcileLiveSessionHeat(
		decodeHeatStateMap(state.Heat),
		live,
		clients,
		observations,
		time.Now().UTC(),
		cfg.halfLife,
		cfg.staleAfter,
	)
	state.Heat = encodeHeatStateMap(nextHeat)
	for sessionName, trace := range traces {
		s.logHeatTrace(sessionName, trace)
	}
	return heatCaptureResult{captured: true, complete: paneObservationsComplete(observations), completeSessions: paneObservationCompleteSessions(live, observations)}
}

func paneObservationCompleteSessions(live []ports.TmuxSessionSnapshot, observations []paneObservation) map[string]bool {
	complete := make(map[string]bool, len(live))
	for _, session := range live {
		complete[session.Name] = true
	}
	for _, observation := range observations {
		if !observation.Sampled && observation.SessionName != "" {
			complete[observation.SessionName] = false
		}
	}
	return complete
}

func paneObservationsComplete(observations []paneObservation) bool {
	for _, observation := range observations {
		if !observation.Sampled {
			return false
		}
	}
	return true
}

func (s *Service) logHeatTrace(sessionName string, trace heat.Trace) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Debug(string(trace.Status), []ports.LogField{
		{Key: "session", Value: sessionName},
		{Key: "status", Value: trace.Status},
		{Key: "bucket", Value: trace.Bucket},
		{Key: "idle_for", Value: trace.IdleFor},
		{Key: "observed_activity", Value: trace.ObservedActivity},
		{Key: "visited", Value: trace.Visited},
	})
}

func heatConfigFromSnapshot(snapshot ports.ConfigSnapshot) heatConfig {
	cfg := heatConfig{halfLife: defaultHeatHalfLife, staleAfter: defaultHeatStaleAfter}
	if snapshot.HeatHalfLifeHours > 0 {
		cfg.halfLife = time.Duration(snapshot.HeatHalfLifeHours) * time.Hour
	}
	if snapshot.HeatStaleHours > 0 {
		cfg.staleAfter = time.Duration(snapshot.HeatStaleHours) * time.Hour
	}
	return cfg
}

func (s *Service) loadHeatConfig(ctx context.Context) heatConfig {
	cfg := heatConfigFromSnapshot(ports.ConfigSnapshot{})
	if s.tmuxConfig == nil {
		return cfg
	}
	snapshot, err := s.tmuxConfig.LoadConfig(ctx)
	if err != nil {
		return cfg
	}
	return heatConfigFromSnapshot(snapshot)
}

func collectPaneObservations(ctx context.Context, query ports.TmuxQueryPort) ([]paneObservation, error) {
	sampler, ok := any(query).(paneActivityQuery)
	if !ok {
		return nil, nil
	}
	panes, err := sampler.ListPanes(ctx)
	if err != nil {
		return nil, err
	}
	observations := make([]paneObservation, 0, len(panes))
	for _, pane := range panes {
		if pane.Sidebar || strings.TrimSpace(pane.PaneID) == "" || strings.TrimSpace(pane.SessionName) == "" {
			continue
		}
		observation := paneObservation{SessionID: pane.SessionID, SessionName: pane.SessionName, PaneID: pane.PaneID}
		text, err := sampler.CapturePaneText(ctx, pane.PaneID, paneSampleTailLines)
		if err == nil {
			observation.Fingerprint = fingerprintPaneText(text)
			observation.Sampled = true
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func fingerprintPaneText(text string) string {
	// SHA-256 gives stable, low-collision pane fingerprints for short sampled text
	// without needing anything more specialized for this change-detection use case.
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func reconcileLiveSessionHeat(current map[string]heat.State, live []ports.TmuxSessionSnapshot, clients []ports.TmuxClientSnapshot, observations []paneObservation, now time.Time, halfLife time.Duration, staleAfter time.Duration) (map[string]heat.State, map[string]heat.Trace) {
	next := make(map[string]heat.State, len(current)+len(live))
	traces := make(map[string]heat.Trace, len(live))
	for name, state := range current {
		next[name] = cloneHeatState(state)
	}

	sessionNamesByID := make(map[string]string, len(live))
	for _, session := range live {
		sessionNamesByID[session.ID] = session.Name
	}
	observationsBySession := make(map[string][]paneObservation, len(live))
	for _, observation := range observations {
		sessionName := strings.TrimSpace(observation.SessionName)
		if sessionName == "" {
			sessionName = sessionNamesByID[observation.SessionID]
		}
		if sessionName == "" {
			continue
		}
		observation.SessionName = sessionName
		observationsBySession[sessionName] = append(observationsBySession[sessionName], observation)
	}
	visited := visitedSessionNames(clients, sessionNamesByID)

	for _, session := range live {
		state := cloneHeatState(next[session.Name])
		active := applyPaneObservations(&state, observationsBySession[session.Name])
		nextState, trace := heat.Advance(state, now, active, visited[session.Name], halfLife, staleAfter)
		next[session.Name] = nextState
		traces[session.Name] = trace
	}
	return next, traces
}

func applyPaneObservations(state *heat.State, observations []paneObservation) bool {
	paneObservations := make([]heat.PaneObservation, 0, len(observations))
	for _, observation := range observations {
		paneObservations = append(paneObservations, heat.PaneObservation{
			PaneID:      observation.PaneID,
			Fingerprint: observation.Fingerprint,
			Sampled:     observation.Sampled,
		})
	}
	return heat.ApplyPaneObservations(state, paneObservations)
}

func visitedSessionNames(clients []ports.TmuxClientSnapshot, sessionNamesByID map[string]string) map[string]bool {
	visited := make(map[string]bool, len(clients))
	for _, client := range clients {
		if !client.Attached {
			continue
		}
		sessionName := sessionNamesByID[client.CurrentSessionID]
		if sessionName != "" {
			visited[sessionName] = true
		}
	}
	return visited
}

func cloneHeatState(state heat.State) heat.State {
	clone := state
	if state.Panes != nil {
		clone.Panes = make(map[string]heat.PaneState, len(state.Panes))
		maps.Copy(clone.Panes, state.Panes)
	}
	return clone
}

func clearTransientHeatState(state heat.State) heat.State {
	state.RecentActivityAt = time.Time{}
	state.LastVisitedAt = time.Time{}
	state.Panes = nil
	return state
}

func decodeHeatStateMap(raw map[string][]byte) map[string]heat.State {
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

func encodeHeatStateMap(states map[string]heat.State) map[string][]byte {
	encoded := make(map[string][]byte, len(states))
	for name, state := range states {
		data, err := json.Marshal(state)
		if err != nil {
			continue
		}
		encoded[name] = data
	}
	return encoded
}
