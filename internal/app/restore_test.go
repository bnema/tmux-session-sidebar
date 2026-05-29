package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type delayedIPCServer struct {
	started chan struct{}
	done    chan struct{}
	delay   time.Duration
}

func (s *delayedIPCServer) Serve(ctx context.Context, _ ports.IPCHandler) error {
	close(s.started)
	<-ctx.Done()
	time.Sleep(s.delay)
	close(s.done)
	return nil
}

func TestDaemonEnsureRestoresMissingPersistedNamedSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	restorePath := t.TempDir()
	seedPersistedState(t, map[string]ports.SessionMetadata{
		"alpha": {Kind: "captured", LastPath: restorePath},
	}, nil)
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "daemon/ensure", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon ensure error: %v", err)
	}

	log := readLog(t, logPath)
	if !strings.Contains(log, "new-session -d -s alpha -c "+restorePath) {
		t.Fatalf("expected daemon ensure to restore alpha at %q, log=%q", restorePath, log)
	}
}

func TestDaemonEnsureDoesNotCreateNumericOrHiddenPersistedSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	restorePath := t.TempDir()
	seedPersistedState(t, map[string]ports.SessionMetadata{
		"123":      {Kind: "captured", LastPath: restorePath},
		"__hidden": {Kind: "captured", LastPath: restorePath},
	}, nil)
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "daemon/ensure", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon ensure error: %v", err)
	}

	log := readLog(t, logPath)
	if strings.Contains(log, "new-session") {
		t.Fatalf("expected daemon ensure not to create numeric or hidden sessions, log=%q", log)
	}
}

func TestDaemonServeCapturesLiveSessionsBeforeWaiting(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
            @session-sidebar-activity-debug-log) printf 'off\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  display-message)
    if [ "$5" = '#{pane_current_path}' ]; then
      printf '%s\n' "$CAPTURE_PATH"
    fi ;;
esac
`)
	t.Setenv("CAPTURE_PATH", alphaPath)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "daemon/serve", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon serve error: %v", err)
	}

	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if got := state.Sessions["alpha"]; got.Kind != "captured" || got.LastPath != alphaPath {
		t.Fatalf("alpha metadata = %#v, want captured at %q", got, alphaPath)
	}
}

func TestDaemonEnsureClearsTransientHeatStateOnStartup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	encodedHeat, err := json.Marshal(heat.State{
		Score:            600,
		UpdatedAt:        time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		LastActiveAt:     time.Date(2026, 5, 23, 11, 55, 0, 0, time.UTC),
		RecentActivityAt: time.Date(2026, 5, 23, 11, 59, 0, 0, time.UTC),
		LastVisitedAt:    time.Date(2026, 5, 23, 11, 58, 0, 0, time.UTC),
		Panes:            map[string]heat.PaneState{"%1": {Fingerprint: "abc"}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	state.Heat = map[string][]byte{"alpha": encodedHeat}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) ;;
  list-clients) ;;
  list-panes) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "daemon/ensure", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon ensure error: %v", err)
	}

	nextState, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	decoded := decodePersistedHeat(nextState.Heat)["alpha"]
	if !decoded.RecentActivityAt.IsZero() {
		t.Fatalf("recent activity at = %v, want zero after startup reset", decoded.RecentActivityAt)
	}
	if !decoded.LastVisitedAt.IsZero() {
		t.Fatalf("last visited at = %v, want zero after startup reset", decoded.LastVisitedAt)
	}
	if len(decoded.Panes) != 0 {
		t.Fatalf("panes = %#v, want cleared transient pane fingerprints after startup reset", decoded.Panes)
	}
}

func TestDaemonServeRefreshesOpenSidebarsAfterHeatTick(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
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
      @session-sidebar-heat-refresh-seconds) printf '1\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes)
    if [ "$3" = '-f' ]; then
      printf '%%sidebar\n'
    else
      printf '%%1\t$1\talpha\t@1\t/tmp/alpha\tpi\t0\t\t0\n'
    fi ;;
  capture-pane) printf 'working\n' ;;
  send-keys) ;;
esac
`)

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "daemon/serve", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon serve error: %v", err)
	}

	if log := readLog(t, logPath); !strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("expected daemon heat tick to refresh open sidebar, log=%q", log)
	}
}

func TestDaemonServeClearsTransientHeatStateOnStartup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	seedTransientHeatState(t)
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
            @session-sidebar-activity-debug-log) printf 'off\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) ;;
  list-clients) ;;
  list-panes) ;;
esac
`)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "daemon/serve", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon serve error: %v", err)
	}

	assertTransientHeatStateCleared(t)
}

func TestDaemonServeWaitsForIPCServerShutdown(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '20\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) ;;
  list-clients) ;;
  list-panes) ;;
esac
`)

	server := &delayedIPCServer{started: make(chan struct{}), done: make(chan struct{}), delay: 25 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := serveSidebarDaemon(ctx, server, runtimeRouter{}); err != nil {
		t.Fatalf("serveSidebarDaemon error: %v", err)
	}
	select {
	case <-server.started:
	default:
		t.Fatal("ipc server never started")
	}
	select {
	case <-server.done:
	default:
		t.Fatal("serveSidebarDaemon returned before ipc server shutdown finished")
	}
}

func TestCaptureLiveSidebarHeatWritesActivityDebugLogWhenEnabled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes) printf '%%1\t$1\talpha\t@1\t/tmp/alpha\tpi\t0\t\t0\n' ;;
  capture-pane) printf 'working\n' ;;
esac
`)

	cfg := ports.ConfigSnapshot{HeatHalfLifeHours: 8, HeatStaleHours: 24, ActivityDebugLog: true}
	if err := captureLiveSidebarHeat(context.Background(), cfg); err != nil {
		t.Fatalf("captureLiveSidebarHeat error: %v", err)
	}

	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	if !strings.Contains(string(content), "status=doing-nothing") {
		t.Fatalf("activity log missing baseline status: %q", string(content))
	}
}

func seedTransientHeatState(t *testing.T) {
	t.Helper()
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	encodedHeat, err := json.Marshal(heat.State{
		Score:            600,
		UpdatedAt:        time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		LastActiveAt:     time.Date(2026, 5, 23, 11, 55, 0, 0, time.UTC),
		RecentActivityAt: time.Date(2026, 5, 23, 11, 59, 0, 0, time.UTC),
		LastVisitedAt:    time.Date(2026, 5, 23, 11, 58, 0, 0, time.UTC),
		Panes:            map[string]heat.PaneState{"%1": {Fingerprint: "abc"}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	state.Heat = map[string][]byte{"alpha": encodedHeat}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func assertTransientHeatStateCleared(t *testing.T) {
	t.Helper()
	nextState, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	decoded := decodePersistedHeat(nextState.Heat)["alpha"]
	if !decoded.RecentActivityAt.IsZero() {
		t.Fatalf("recent activity at = %v, want zero after startup reset", decoded.RecentActivityAt)
	}
	if !decoded.LastVisitedAt.IsZero() {
		t.Fatalf("last visited at = %v, want zero after startup reset", decoded.LastVisitedAt)
	}
	if len(decoded.Panes) != 0 {
		t.Fatalf("panes = %#v, want cleared transient pane fingerprints after startup reset", decoded.Panes)
	}
}

func TestHookClientAttachedCapturesVisitedAgentAttentionBaseline(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
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
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  list-clients) printf '/dev/pts/1\t$1\t@1\t%%pane\t/dev/pts/1\n' ;;
  display-message) printf '%s\n' "$CAPTURE_PATH" ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)
	t.Setenv("CAPTURE_PATH", alphaPath)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "hook/client-attached", Flags: map[string]string{"client": "/dev/pts/1"}}, nil, nil); err != nil {
		t.Fatalf("hook client-attached error: %v", err)
	}

	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	raw := state.Clients["/dev/pts/1"]
	if len(raw) == 0 {
		t.Fatalf("client baseline missing from persisted state: %#v", state.Clients)
	}
	var clientState struct {
		CurrentSessionID string `json:"currentSessionId,omitempty"`
	}
	if err := json.Unmarshal(raw, &clientState); err != nil {
		t.Fatalf("unmarshal persisted client state: %v", err)
	}
	if clientState.CurrentSessionID != "$1" {
		t.Fatalf("current session id = %q, want $1", clientState.CurrentSessionID)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("expected hook client-attached to refresh open sidebars, log=%q", log)
	}
}

func TestHookClientSessionChangedRefreshesAllOpenSidebars(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  display-message) printf '%s\n' "$CAPTURE_PATH" ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)
	t.Setenv("CAPTURE_PATH", alphaPath)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "hook/client-session-changed", Flags: map[string]string{"client": "%1"}}, nil, nil); err != nil {
		t.Fatalf("hook client-session-changed error: %v", err)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("expected hook client-session-changed to refresh open sidebars, log=%q", log)
	}
}

func TestHookClientSessionChangedStillRefreshesSidebarsWhenListClientsFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
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
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  list-clients) exit 1 ;;
  display-message) printf '%s\n' "$CAPTURE_PATH" ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)
	t.Setenv("CAPTURE_PATH", alphaPath)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "hook/client-session-changed", Flags: map[string]string{"client": "%1"}}, nil, nil); err != nil {
		t.Fatalf("hook client-session-changed error: %v", err)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("expected refresh despite list-clients failure, log=%q", log)
	}
}

func TestHookClientSessionChangedReconcilesLiveNamedSessionsAndPrunesAbsentRecords(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	betaPath := t.TempDir()
	seedPersistedState(t, map[string]ports.SessionMetadata{
		"beta": {Kind: "project", ProjectPath: betaPath, LastPath: betaPath},
	}, []string{"beta"})
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  display-message) printf '%s\n' "$CAPTURE_PATH" ;;
esac
`)
	t.Setenv("CAPTURE_PATH", alphaPath)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "hook/client-session-changed", Flags: map[string]string{"client": "%1"}}, nil, nil); err != nil {
		t.Fatalf("hook client-session-changed error: %v", err)
	}

	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if got := state.Sessions["alpha"]; got.Kind != "captured" || got.LastPath != alphaPath {
		t.Fatalf("alpha metadata = %#v, want captured at %q", got, alphaPath)
	}
	if _, ok := state.Sessions["beta"]; ok {
		t.Fatalf("beta metadata still present after reconcile: %#v", state.Sessions)
	}
	if len(state.SessionOrder) != 1 || state.SessionOrder[0] != "alpha" {
		t.Fatalf("SessionOrder = %#v, want []string{\"alpha\"}", state.SessionOrder)
	}
}
