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

	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/core/persisted"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/internal/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
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

// paneActivityQuery is an optional extension over ports.QueryPort that enables
// pane sampling. collectPaneObservations gracefully degrades to nil observations when
// the query does not implement it.
type paneActivityQuery interface {
	ListPanes(ctx context.Context) ([]ports.PaneSnapshot, error)
	CapturePaneText(ctx context.Context, paneID string, tailLines int) (string, error)
}

type heatConfig struct {
	halfLife   time.Duration
	staleAfter time.Duration
}

type heatCaptureResult struct {
	captured         bool
	complete         bool
	observedActivity bool
	completeSessions map[string]bool
}

type recentSessionOrderMode int

const (
	recentSessionOrderSkip recentSessionOrderMode = iota
	recentSessionOrderApply
	recentSessionOrderPreserve
)

func SessionHeatCaptureRequired(snapshot ports.ConfigSnapshot) bool {
	return snapshot.HeatColorsEnabled || snapshot.AutoSortRecentInterval > 0
}

func (s *Service) RestorePersistedSessions(ctx context.Context, serverID string, home string) RestoreReport {
	report := RestoreReport{Failed: map[string]error{}, SystemFailures: map[string]error{}}
	if s.store == nil {
		report.SystemFailures["store"] = ErrMissingStateStore
		return report
	}
	if s.query == nil {
		report.SystemFailures["query"] = ErrMissingQuery
		return report
	}
	if s.control == nil {
		report.SystemFailures["control"] = ErrMissingControl
		return report
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		report.SystemFailures["load"] = err
		return report
	}
	live, err := s.query.ListSessions(ctx)
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
		if err := s.control.CreateSession(ctx, name, path); err != nil {
			report.Failed[name] = err
			continue
		}
		report.Restored = append(report.Restored, name)
	}
	return report
}

func (s *Service) CaptureLiveSessions(ctx context.Context, serverID string) error {
	_, err := s.captureLiveSessions(ctx, serverID, captureLiveSessionsOptions{})
	return err
}

func (s *Service) CaptureLiveSessionsProtected(ctx context.Context, serverID string) (bool, error) {
	return s.captureLiveSessions(ctx, serverID, captureLiveSessionsOptions{protectEmpty: true})
}

func (s *Service) CaptureSessionHeat(ctx context.Context, serverID string) error {
	if s.store == nil {
		return ErrMissingStateStore
	}
	if s.query == nil {
		return ErrMissingQuery
	}

	live, err := s.query.ListSessions(ctx)
	if err != nil {
		return err
	}
	return ports.UpdateState(ctx, s.store, serverID, func(state *ports.PersistedState) error {
		_ = s.captureHeatIntoState(ctx, state, live, s.loadHeatConfig(ctx))
		return nil
	})
}

func (s *Service) CaptureLiveSessionsWithConfig(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) error {
	_, err := s.captureLiveSessions(ctx, serverID, captureLiveSessionsOptions{snapshot: &snapshot, recentSessionOrder: recentSessionOrderApply})
	return err
}

func (s *Service) CaptureLiveSessionsWithConfigProtected(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) (bool, error) {
	return s.captureLiveSessions(ctx, serverID, captureLiveSessionsOptions{snapshot: &snapshot, protectEmpty: true, recentSessionOrder: recentSessionOrderApply})
}

func (s *Service) CaptureLiveSessionsWithConfigProtectedPreservingOrder(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) (bool, error) {
	return s.captureLiveSessions(ctx, serverID, captureLiveSessionsOptions{snapshot: &snapshot, protectEmpty: true, recentSessionOrder: recentSessionOrderPreserve, requireCompleteHeatCapture: true})
}

type captureLiveSessionsOptions struct {
	snapshot                   *ports.ConfigSnapshot
	protectEmpty               bool
	recentSessionOrder         recentSessionOrderMode
	requireCompleteHeatCapture bool
}

// captureLiveSessions is the shared implementation for all CaptureLiveSessions variants.
// snapshot controls heat config: non-nil activates captureHeatIntoState with
// the provided config, while nil loads heat config from multiplexer and skips
// recent-session-order. recentSessionOrder controls whether a config-based
// capture may rewrite SessionOrder, skip sorting, or preserve startup order.
// protectEmpty controls whether a destructive-empty skip is applied: when true and no
// persistable sessions are live but the persisted state is meaningful, the capture is
// skipped to guard against overwriting restored state during startup/restore windows.
func (s *Service) captureLiveSessions(ctx context.Context, serverID string, opts captureLiveSessionsOptions) (bool, error) {
	if s.store == nil {
		return false, ErrMissingStateStore
	}
	if s.query == nil {
		return false, ErrMissingQuery
	}

	live, err := s.query.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	err = ports.UpdateState(ctx, s.store, serverID, func(state *ports.PersistedState) error {
		if opts.protectEmpty && persistableSessionCount(live) == 0 && persisted.IsMeaningful(*state) {
			return nil
		}
		state.Sessions = reconcileLiveSessionMetadata(ctx, s.query, live, state.Sessions)
		state.SessionOrder = reconcileSessionOrder(state.SessionOrder, live)
		state.PinnedSessions = reconcilePinnedSessions(state.PinnedSessions, live)
		state.PinColors = reconcilePinColors(state.PinColors, live)
		if opts.snapshot != nil {
			capture := heatCaptureResult{}
			heatRequired := SessionHeatCaptureRequired(*opts.snapshot)
			previousHeat := maps.Clone(state.Heat)
			now := s.now()
			if heatRequired {
				capture = s.captureHeatIntoStateAt(ctx, state, live, heatConfigFromSnapshot(*opts.snapshot), now.UTC())
			}
			if opts.requireCompleteHeatCapture && heatRequired && !capture.complete {
				state.Heat = previousHeat
			}
			switch opts.recentSessionOrder {
			case recentSessionOrderApply:
				applyRecentSessionOrderAfterCapture(state, live, *opts.snapshot, now, capture)
			case recentSessionOrderPreserve:
				// Startup baseline captures intentionally preserve restored order and do not
				// consume the user's auto-sort interval. The first later capture with real
				// pane activity is still eligible to sort immediately.
			case recentSessionOrderSkip:
			}
			changed = !opts.requireCompleteHeatCapture || !heatRequired || capture.complete
		} else {
			_ = s.captureHeatIntoState(ctx, state, live, s.loadHeatConfig(ctx))
			changed = true
		}
		return nil
	})
	return changed, err
}

func (s *Service) CaptureSessionHeatWithConfig(ctx context.Context, serverID string, snapshot ports.ConfigSnapshot) error {
	if !SessionHeatCaptureRequired(snapshot) {
		return nil
	}
	if s.store == nil {
		return ErrMissingStateStore
	}
	if s.query == nil {
		return ErrMissingQuery
	}

	live, err := s.query.ListSessions(ctx)
	if err != nil {
		return err
	}
	return ports.UpdateState(ctx, s.store, serverID, func(state *ports.PersistedState) error {
		now := s.now()
		capture := s.captureHeatIntoStateAt(ctx, state, live, heatConfigFromSnapshot(snapshot), now.UTC())
		applyRecentSessionOrderAfterCapture(state, live, snapshot, now, capture)
		return nil
	})
}

func (s *Service) ResetTransientHeatState(ctx context.Context, serverID string) error {
	return s.resetTransientHeatState(ctx, serverID, nil)
}

func (s *Service) ResetTransientHeatStateForSessions(ctx context.Context, serverID string, sessionNames []string) error {
	if len(sessionNames) == 0 {
		return nil
	}
	only := make(map[string]bool, len(sessionNames))
	for _, name := range sessionNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		only[trimmed] = true
	}
	if len(only) == 0 {
		return nil
	}
	return s.resetTransientHeatState(ctx, serverID, only)
}

func (s *Service) resetTransientHeatState(ctx context.Context, serverID string, only map[string]bool) error {
	if s.store == nil {
		return ErrMissingStateStore
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return fmt.Errorf("reset transient heat state: load: %w", err)
	}
	decoded := decodeHeatStateMap(state.Heat)
	for name, heatState := range decoded {
		if only != nil && !only[name] {
			continue
		}
		decoded[name] = clearTransientHeatState(heatState)
	}
	state.Heat = encodeHeatStateMap(decoded)
	if err := s.store.Save(ctx, serverID, state); err != nil {
		return fmt.Errorf("reset transient heat state: save: %w", err)
	}
	return nil
}

func reconcileLiveSessionMetadata(ctx context.Context, query ports.QueryPort, live []ports.SessionSnapshot, current map[string]ports.SessionMetadata) map[string]ports.SessionMetadata {
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

func reconcileSessionOrder(order []string, live []ports.SessionSnapshot) []string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ApplyOrder(liveNames, order)
}

func reconcilePinnedSessions(pinned []string, live []ports.SessionSnapshot) []string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ReconcilePinned(pinned, liveNames)
}

func reconcilePinColors(colors map[string]string, live []ports.SessionSnapshot) map[string]string {
	liveNames := make([]string, 0, len(live))
	for _, session := range live {
		liveNames = append(liveNames, session.Name)
	}
	return sessions.ReconcileNamedStrings(colors, liveNames)
}

// persistableSessionCount returns the number of live sessions whose name
// qualifies for persistence (valid, non-hidden, non-numeric).
func persistableSessionCount(live []ports.SessionSnapshot) int {
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

func applyRecentSessionOrder(state *ports.PersistedState, live []ports.SessionSnapshot, cfg ports.ConfigSnapshot, now time.Time) {
	applyRecentSessionOrderAfterCapture(state, live, cfg, now, heatCaptureResult{captured: true, complete: true, observedActivity: true})
}

func applyRecentSessionOrderAfterCapture(state *ports.PersistedState, live []ports.SessionSnapshot, cfg ports.ConfigSnapshot, now time.Time, capture heatCaptureResult) {
	if !recentSessionOrderCaptureDue(state, cfg, now, capture) {
		return
	}
	ordered := orderSessionsByRecentActivityPinned(state.SessionOrder, live, decodeHeatStateMap(state.Heat), state.PinnedSessions)
	if !capture.complete {
		reorderCompleteSidebarLayoutCategories(state.SidebarLayout, ordered, state.PinnedSessions, capture.completeSessions)
		return
	}
	state.SessionOrder = ordered
	addMissingSidebarLayoutSessionsToDefault(state.SidebarLayout, state.SessionOrder)
	reorderSidebarLayoutCategories(state.SidebarLayout, state.SessionOrder, state.PinnedSessions)
	markRecentSessionOrderRun(state, now)
}

func recentSessionOrderCaptureDue(state *ports.PersistedState, cfg ports.ConfigSnapshot, now time.Time, capture heatCaptureResult) bool {
	if state == nil || cfg.AutoSortRecentInterval <= 0 || !capture.captured {
		return false
	}
	lastRunAt, hadLastRun := time.Time{}, false
	if state.Sidebar != nil {
		lastRunAt, hadLastRun = autoSortRecentLastRunAt(*state.Sidebar)
		if hadLastRun && now.Sub(lastRunAt) < cfg.AutoSortRecentInterval {
			return false
		}
	}
	return capture.observedActivity || heatActivityAfter(state.Heat, lastRunAt, hadLastRun)
}

func markRecentSessionOrderRun(state *ports.PersistedState, now time.Time) {
	if state.Sidebar == nil {
		state.Sidebar = &ports.SidebarState{}
	}
	state.Sidebar.AutoSortRecentRunAt = now.Format(time.RFC3339Nano)
	state.Sidebar.AutoSortRecentRunDate = ""
}

func reorderSidebarLayoutCategories(layout *ports.SidebarLayout, order []string, pinned []string) {
	reorderSidebarLayoutCategoriesWhere(layout, order, pinned, nil)
}

func addMissingSidebarLayoutSessionsToDefault(layout *ports.SidebarLayout, order []string) {
	if layout == nil {
		return
	}
	assigned := make(map[string]bool)
	defaultIndex := -1
	for i, item := range layout.Items {
		category := item.Category
		if category == nil {
			continue
		}
		if item.ID == sidebarlayout.DefaultCategoryID || category.ID == sidebarlayout.DefaultCategoryID {
			defaultIndex = i
		}
		for _, ref := range category.Sessions {
			assigned[strings.TrimSpace(ref.Name)] = true
		}
	}
	missing := make([]ports.SidebarLayoutSessionRef, 0, len(order))
	for _, name := range order {
		name = strings.TrimSpace(name)
		if assigned[name] || !sessions.IsPersistableName(name) {
			continue
		}
		missing = append(missing, ports.SidebarLayoutSessionRef{Name: name})
		assigned[name] = true
	}
	if len(missing) == 0 {
		return
	}
	if defaultIndex < 0 {
		layout.Items = append(layout.Items, ports.SidebarLayoutItem{
			ID:   sidebarlayout.DefaultCategoryID,
			Kind: string(sidebarlayout.ItemKindCategory),
			Category: &ports.SidebarLayoutCategory{
				ID:   sidebarlayout.DefaultCategoryID,
				Name: sidebarlayout.DefaultCategoryName,
			},
		})
		defaultIndex = len(layout.Items) - 1
	}
	category := layout.Items[defaultIndex].Category
	category.Sessions = append(category.Sessions, missing...)
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

func heatActivityAfter(raw map[string][]byte, since time.Time, ok bool) bool {
	if !ok {
		return false
	}
	for _, state := range decodeHeatStateMap(raw) {
		if state.LastActiveAt.After(since) {
			return true
		}
	}
	return false
}

func orderSessionsByRecentActivityPinned(order []string, live []ports.SessionSnapshot, heatStates map[string]heat.State, pinned []string) []string {
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

func (s *Service) captureHeatIntoState(ctx context.Context, state *ports.PersistedState, live []ports.SessionSnapshot, cfg heatConfig) heatCaptureResult {
	return s.captureHeatIntoStateAt(ctx, state, live, cfg, s.now().UTC())
}

func (s *Service) captureHeatIntoStateAt(ctx context.Context, state *ports.PersistedState, live []ports.SessionSnapshot, cfg heatConfig, now time.Time) heatCaptureResult {
	if state == nil {
		return heatCaptureResult{}
	}
	clients, clientsErr := s.query.ListClients(ctx)
	observations, observationsErr := collectPaneObservations(ctx, s.query)
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
		now,
		cfg.halfLife,
		cfg.staleAfter,
	)
	state.Heat = encodeHeatStateMap(nextHeat)
	for sessionName, trace := range traces {
		s.logHeatTrace(sessionName, trace)
	}
	return heatCaptureResult{captured: true, complete: paneObservationsComplete(observations), observedActivity: heatTraceObservedActivity(traces), completeSessions: paneObservationCompleteSessions(live, observations)}
}

func heatTraceObservedActivity(traces map[string]heat.Trace) bool {
	for _, trace := range traces {
		if trace.ObservedActivity {
			return true
		}
	}
	return false
}

func paneObservationCompleteSessions(live []ports.SessionSnapshot, observations []paneObservation) map[string]bool {
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

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
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
	if s.config == nil {
		return cfg
	}
	snapshot, err := s.config.LoadConfig(ctx)
	if err != nil {
		return cfg
	}
	return heatConfigFromSnapshot(snapshot)
}

func collectPaneObservations(ctx context.Context, query ports.QueryPort) ([]paneObservation, error) {
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

func reconcileLiveSessionHeat(current map[string]heat.State, live []ports.SessionSnapshot, clients []ports.ClientSnapshot, observations []paneObservation, now time.Time, halfLife time.Duration, staleAfter time.Duration) (map[string]heat.State, map[string]heat.Trace) {
	next := make(map[string]heat.State, len(live))
	traces := make(map[string]heat.Trace, len(live))

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
		state := cloneHeatState(current[session.Name])
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

func visitedSessionNames(clients []ports.ClientSnapshot, sessionNamesByID map[string]string) map[string]bool {
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
