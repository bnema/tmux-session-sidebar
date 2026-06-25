package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
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

func TestDaemonEnsureSkipsLightweightRestoreDuringContinuumStartupWindow(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	restorePath := t.TempDir()
	seedPersistedState(t, map[string]ports.SessionMetadata{
		"alpha": {Kind: "captured", LastPath: restorePath},
	}, nil)
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "${@: -1}" in
      @continuum-restore) printf 'on\n' ;;
      @continuum-restore-max-delay) printf '10\n' ;;
      @resurrect-restore-script-path) printf '/tmp/resurrect/restore.sh\n' ;;
      @session-sidebar-restore-sessions) printf 'auto\n' ;;
      @session-sidebar-continuum-grace-seconds) printf '3\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message)
    case "${@: -1}" in
      '#{start_time}') printf '%s\n' "$(date +%s)" ;;
    esac ;;
  list-sessions) ;;
  list-clients) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "daemon/ensure", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon ensure error: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "new-session -d -s alpha") {
		t.Fatalf("expected daemon ensure not to create alpha during continuum restore window, log=%q", log)
	}
	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if _, ok := state.Sessions["alpha"]; !ok {
		t.Fatalf("persisted sessions were truncated during continuum restore window: %#v", state.Sessions)
	}
}

func TestDaemonEnsureRestoreSessionsOnOverridesContinuumStartupWindow(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	restorePath := t.TempDir()
	seedPersistedState(t, map[string]ports.SessionMetadata{
		"alpha": {Kind: "captured", LastPath: restorePath},
	}, nil)
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    if [ "$2" = "-g" ] && [ $# -eq 2 ]; then
      printf '@session-sidebar-key b\n'
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
      printf '@session-sidebar-agent-attention on\n'
      printf '@session-sidebar-agent-attention-animation pulse\n'
      printf '@session-sidebar-auto-sort-recent off\n'
      printf '@session-sidebar-restore-sessions on\n'
      printf '@session-sidebar-continuum-grace-seconds 3\n'
      printf '@session-sidebar-metadata-subline off\n'
      printf '@session-sidebar-metadata-inactive off\n'
    else
      case "${@: -1}" in
        @continuum-restore) printf 'on\n' ;;
        @continuum-restore-max-delay) printf '10\n' ;;
        @resurrect-restore-script-path) printf '/tmp/resurrect/restore.sh\n' ;;
        @session-sidebar-restore-sessions) printf 'on\n' ;;
        @session-sidebar-continuum-grace-seconds) printf '3\n' ;;
        *) printf '\n' ;;
      esac
    fi ;;
  display-message)
    case "${@: -1}" in
      '#{start_time}') printf '%s\n' "$(date +%s)" ;;
    esac ;;
  list-sessions) ;;
  list-clients) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "daemon/ensure", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon ensure error: %v", err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "new-session -d -s alpha -c "+restorePath) {
		t.Fatalf("expected restore-sessions=on to create alpha, log=%q", log)
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
      @session-sidebar-width) printf '30\n' ;;
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

func TestDaemonServeStartsMetadataWatcherWhenConfigEnablesAfterStartup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	counterPath := filepath.Join(t.TempDir(), "metadata-config-calls")
	alphaPath := t.TempDir()
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '1\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'off\n' ;;
      @session-sidebar-metadata-subline)
        count=$(cat "$METADATA_CONFIG_CALLS" 2>/dev/null || printf '0')
        count=$((count + 1))
        printf '%s\n' "$count" > "$METADATA_CONFIG_CALLS"
        if [ "$count" -le 1 ]; then printf 'off\n'; else printf 'on\n'; fi ;;
      @session-sidebar-metadata-inactive) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes)
    if [ "$2" = "-a" ]; then printf 'alpha\t1\t1\t%s\n' "$CAPTURE_PATH"; fi ;;
  display-message) printf '%s\n' "$CAPTURE_PATH" ;;
esac
`)
	t.Setenv("METADATA_CONFIG_CALLS", counterPath)
	t.Setenv("CAPTURE_PATH", alphaPath)
	watcher := newMetadataLifecycleWatcher()
	git := metadataFakeGit{
		infos:    map[string]ports.GitRepoInfo{alphaPath: {RepoRoot: alphaPath, WorktreeRoot: alphaPath, GitDir: filepath.Join(alphaPath, ".git"), CommonGitDir: filepath.Join(alphaPath, ".git")}},
		targets:  map[string]ports.GitWatchTargets{alphaPath: {RepoRoot: alphaPath, WorktreeRoot: alphaPath, Files: []string{filepath.Join(alphaPath, ".git", "HEAD")}, Dirs: []string{alphaPath}}},
		statuses: map[string]ports.GitStatus{alphaPath: {RepoRoot: alphaPath, Branch: "main"}},
	}
	deps := runtimeDependencies()
	updatedDeps := deps
	updatedDeps.Git = git
	updatedDeps.WatcherFactory = func() ports.FileWatcherPort { return watcher }
	SetRuntimeDependencies(updatedDeps)
	t.Cleanup(func() { SetRuntimeDependencies(deps) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveSidebarDaemonWithOptions(ctx, nil, nil, daemonServeOptions{}) }()
	watcher.waitStarts(t, 1)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serveSidebarDaemonWithOptions error: %v", err)
	}
}

func TestMetadataWatcherRestartInCooldown(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	if metadataWatcherRestartInCooldown(now, time.Time{}) {
		t.Fatal("cooldown active without a recorded failure")
	}
	if !metadataWatcherRestartInCooldown(now, now.Add(-defaultMetadataCaptureFailureCooldown+time.Nanosecond)) {
		t.Fatal("cooldown inactive before restart cooldown elapsed")
	}
	if metadataWatcherRestartInCooldown(now, now.Add(-defaultMetadataCaptureFailureCooldown)) {
		t.Fatal("cooldown active after restart cooldown elapsed")
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

func TestDaemonServeDefersLiveSessionCaptureUntilContinuumWindowEnds(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sessions = map[string]ports.SessionMetadata{
		"alpha": {Kind: "captured", LastPath: "/tmp/alpha"},
		"beta":  {Kind: "captured", LastPath: "/tmp/beta"},
	}
	state.SessionOrder = []string{"alpha", "beta"}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	invocationFile := filepath.Join(t.TempDir(), "list-sessions-count")
	t.Setenv("TEST_INVOCATION_FILE", invocationFile)
	t.Setenv("TEST_START", fmt.Sprintf("%d", time.Now().Unix()))
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "${@: -1}" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      @session-sidebar-auto-sort-recent) printf 'off\n' ;;
      @session-sidebar-restore-sessions) printf 'auto\n' ;;
      @session-sidebar-continuum-grace-seconds) printf '0\n' ;;
      @continuum-restore) printf 'on\n' ;;
      @continuum-restore-max-delay) printf '1\n' ;;
      @resurrect-restore-script-path) printf '/tmp/resurrect/restore.sh\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message)
    case "${@: -1}" in
      '#{start_time}') printf '%s\n' "$TEST_START" ;;
      '#{pane_current_path}') printf '/tmp/%s\n' "${3#=}" ;;
    esac ;;
  list-sessions)
    count=0
    [ -f "$TEST_INVOCATION_FILE" ] && count="$(cat "$TEST_INVOCATION_FILE")"
    count=$((count + 1))
    printf '%s\n' "$count" > "$TEST_INVOCATION_FILE"
    if [ "$count" -eq 1 ]; then
      printf '$1\talpha\t1\t0\n'
    else
      printf '$1\talpha\t1\t0\n$2\tbeta\t1\t0\n'
    fi ;;
  list-clients) ;;
  list-panes) ;;
esac
`)

	// Simulate the partial live server that would have been observed by an
	// immediate startup capture; the daemon should preserve persisted beta until
	// its post-Continuum capture runs and sees the second invocation.
	if _, err := tmux(context.Background(), "list-sessions"); err != nil {
		t.Fatalf("prime fake list-sessions invocation: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := (runtimeRouter{}).Handle(ctx, Route{Path: "daemon/serve", Flags: map[string]string{}}, nil, nil); err != nil {
		t.Fatalf("daemon serve error: %v", err)
	}
	next, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if _, ok := next.Sessions["beta"]; !ok {
		t.Fatalf("beta was removed instead of being preserved until post-window capture: %#v", next.Sessions)
	}
}

func TestDaemonServeRefreshesOpenSidebarsAfterHeatTick(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
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
      printf '@session-sidebar-heat-refresh-seconds 1\n'
      printf '@session-sidebar-heat-recent 1h\n'
      printf '@session-sidebar-heat-max-highlighted 0\n'
      printf '@session-sidebar-activity-debug-log off\n'
      printf '@session-sidebar-agent-attention on\n'
      printf '@session-sidebar-agent-attention-animation pulse\n'
      printf '@session-sidebar-auto-sort-recent off\n'
      printf '@session-sidebar-restore-sessions off\n'
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
        @session-sidebar-heat-refresh-seconds) printf '1\n' ;;
        @session-sidebar-heat-recent) printf '1h\n' ;;
        @session-sidebar-heat-max-highlighted) printf '0\n' ;;
        @session-sidebar-activity-debug-log) printf 'off\n' ;;
        @session-sidebar-agent-attention) printf 'on\n' ;;
        *) printf '\n' ;;
      esac
    fi ;;
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

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- (runtimeRouter{}).Handle(ctx, Route{Path: "daemon/serve", Flags: map[string]string{}}, nil, nil)
	}()
	defer func() {
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("daemon serve error: %v", err)
		}
	}()

	waitForLogContains(t, logPath, "send-keys -t %sidebar F5")
}

func waitForLogContains(t *testing.T, path string, needle string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logBytes, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(logBytes), needle) {
			return
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read fake tmux log: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in fake tmux log: %q", needle, readLog(t, path))
}

func TestFirstReopenAfterTmuxKillServerDoesNotHeatRestoredSessions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	alphaPath := t.TempDir()
	betaPath := t.TempDir()
	gammaPath := t.TempDir()
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	base := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	state.Sessions = map[string]ports.SessionMetadata{
		"alpha": {Kind: "captured", LastPath: alphaPath},
		"beta":  {Kind: "captured", LastPath: betaPath},
		"gamma": {Kind: "captured", LastPath: gammaPath},
	}
	state.SessionOrder = []string{"gamma", "beta", "alpha"}
	state.Heat = map[string][]byte{
		"alpha": mustMarshalHeatState(t, heat.State{UpdatedAt: base, LastActiveAt: base.Add(-3 * time.Hour), Panes: map[string]heat.PaneState{"%old-alpha": {Fingerprint: "old-alpha"}}}),
		"beta":  mustMarshalHeatState(t, heat.State{UpdatedAt: base, LastActiveAt: base.Add(-2 * time.Hour), Panes: map[string]heat.PaneState{"%old-beta": {Fingerprint: "old-beta"}}}),
		"gamma": mustMarshalHeatState(t, heat.State{UpdatedAt: base, LastActiveAt: base.Add(-10 * time.Minute)}),
	}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	listCountPath := filepath.Join(t.TempDir(), "list-sessions-count")
	t.Setenv("TEST_LIST_COUNT", listCountPath)
	t.Setenv("ALPHA_PATH", alphaPath)
	t.Setenv("BETA_PATH", betaPath)
	t.Setenv("GAMMA_PATH", gammaPath)
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "${@: -1}" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-heat-recent) printf '1h\n' ;;
      @session-sidebar-heat-max-highlighted) printf '0\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      @session-sidebar-agent-attention-animation) printf 'pulse\n' ;;
      @session-sidebar-auto-sort-recent) printf '24h\n' ;;
      @session-sidebar-restore-sessions) printf 'on\n' ;;
      @session-sidebar-continuum-grace-seconds) printf '0\n' ;;
      @session-sidebar-color-scheme) printf 'system\n' ;;
      @session-sidebar-metadata-subline) printf 'off\n' ;;
      @session-sidebar-metadata-inactive) printf 'off\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions)
    count=0
    [ -f "$TEST_LIST_COUNT" ] && count="$(cat "$TEST_LIST_COUNT")"
    count=$((count + 1))
    printf '%s\n' "$count" > "$TEST_LIST_COUNT"
    if [ "$count" -eq 1 ]; then
      printf '$3\tgamma\t1\t0\n'
    else
      printf '$3\tgamma\t1\t0\n$1\talpha\t1\t0\n$2\tbeta\t1\t0\n'
    fi ;;
  new-session) ;;
  list-clients) ;;
  list-panes) printf '%%gamma\t$3\tgamma\t@gamma\t%s\tvim\t0\t\t0\n%%alpha-new\t$1\talpha\t@alpha\t%s\tvim\t0\t\t0\n%%beta-new\t$2\tbeta\t@beta\t%s\tvim\t0\t\t0\n' "$GAMMA_PATH" "$ALPHA_PATH" "$BETA_PATH" ;;
  capture-pane)
    case "$*" in
      *%alpha-new*) printf 'alpha restored baseline\n' ;;
      *%beta-new*) printf 'beta restored baseline\n' ;;
      *%gamma*) printf 'gamma unchanged\n' ;;
    esac ;;
  display-message)
    case "$*" in
      *'#{pane_current_path}'*)
        case "$*" in
          *alpha*) printf '%s\n' "$ALPHA_PATH" ;;
          *beta*) printf '%s\n' "$BETA_PATH" ;;
          *gamma*) printf '%s\n' "$GAMMA_PATH" ;;
          *) printf '%s\n' "$GAMMA_PATH" ;;
        esac ;;
      *) printf 'bad-config\n' ;;
    esac ;;
esac
`)

	if err := ensureRestoredAndCaptured(context.Background()); err != nil {
		t.Fatalf("ensureRestoredAndCaptured error: %v", err)
	}

	next, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if got, want := next.SessionOrder, []string{"gamma", "beta", "alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionOrder = %#v, want prior recent order %#v", got, want)
	}
	decoded := decodePersistedHeat(next.Heat)
	if got, want := decoded["alpha"].LastActiveAt, base.Add(-3*time.Hour); !got.Equal(want) {
		t.Fatalf("alpha LastActiveAt = %v, want restored baseline to preserve %v", got, want)
	}
	if got, want := decoded["beta"].LastActiveAt, base.Add(-2*time.Hour); !got.Equal(want) {
		t.Fatalf("beta LastActiveAt = %v, want restored baseline to preserve %v", got, want)
	}
}

func mustMarshalHeatState(t *testing.T, state heat.State) []byte {
	t.Helper()
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return encoded
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
      @session-sidebar-width) printf '30\n' ;;
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
      @session-sidebar-width) printf '30\n' ;;
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
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveSidebarDaemon(ctx, server, runtimeRouter{})
	}()
	select {
	case <-server.started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("ipc server never started")
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveSidebarDaemon error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveSidebarDaemon did not stop after context cancellation")
	}
	select {
	case <-server.done:
	default:
		t.Fatal("serveSidebarDaemon returned before ipc server shutdown finished")
	}
}

func TestCaptureLiveSidebarHeatSkipsWhenNoFeatureNeedsHeat(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  *) ;;
esac
`)

	captured, err := captureLiveSidebarHeat(context.Background(), ports.ConfigSnapshot{})
	if err != nil {
		t.Fatalf("captureLiveSidebarHeat error: %v", err)
	}
	if captured {
		t.Fatal("captureLiveSidebarHeat captured = true, want false when heat colors and auto-sort are disabled")
	}
	if content, err := os.ReadFile(logPath); err == nil && strings.TrimSpace(string(content)) != "" {
		t.Fatalf("heat capture touched tmux despite disabled features, log=%q", string(content))
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read fake tmux log: %v", err)
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

	cfg := ports.ConfigSnapshot{HeatColorsEnabled: true, HeatHalfLifeHours: 8, HeatStaleHours: 24, ActivityDebugLog: true}
	captured, err := captureLiveSidebarHeat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("captureLiveSidebarHeat error: %v", err)
	}
	if !captured {
		t.Fatal("captureLiveSidebarHeat captured = false, want true")
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

func TestCaptureLiveSidebarHeatRotatesOversizedActivityDebugLog(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes) printf '%%1\t$1\talpha\t@1\t/tmp/alpha\tpi\t0\t\t0\n' ;;
  capture-pane) printf 'working\n' ;;
esac
`)
	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatalf("mkdir activity log dir: %v", err)
	}
	oversized := strings.Repeat("old-log-line\n", 120_000)
	if err := os.WriteFile(logPath, []byte(oversized), 0o600); err != nil {
		t.Fatalf("seed oversized activity log: %v", err)
	}

	cfg := ports.ConfigSnapshot{HeatColorsEnabled: true, HeatHalfLifeHours: 8, HeatStaleHours: 24, ActivityDebugLog: true}
	captured, err := captureLiveSidebarHeat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("captureLiveSidebarHeat error: %v", err)
	}
	if !captured {
		t.Fatal("captureLiveSidebarHeat captured = false, want true")
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	if strings.Contains(string(content), "old-log-line") {
		t.Fatalf("activity log still contains rotated content")
	}
	if !strings.Contains(string(content), "status=doing-nothing") {
		t.Fatalf("activity log missing fresh trace after rotation: %q", string(content))
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
      @session-sidebar-width) printf '30\n' ;;
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

func TestHookClientAttachedDoesNotAdoptSidebarForInternalPopupSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "/dev/old"}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "${@: -1}" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\t__floating-popup-1\t1\t1\n' ;;
  list-clients) printf '/dev/popup\t$1\t@popup\t%%popup\t__floating-popup-1\n' ;;
  display-message) printf '/tmp/popup\n' ;;
  list-panes) ;;
esac
`)

	if err := (runtimeRouter{sidebar: runtimeMultiplexer()}).Handle(context.Background(), Route{Path: "hook/client-attached", Flags: map[string]string{"client": "/dev/popup", "session": "__floating-popup-1"}}, nil, nil); err != nil {
		t.Fatalf("hook client-attached error: %v\nlog=%q", err, readLog(t, logPath))
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "new-session -d -s __tmux-session-sidebar") || strings.Contains(log, "join-pane") || strings.Contains(log, "select-pane -t") {
		t.Fatalf("internal popup attach should not open/adopt sidebar, log=%q", log)
	}
	next, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if next.Sidebar == nil || next.Sidebar.OwnerClient != "/dev/old" {
		t.Fatalf("sidebar owner changed for internal popup attach: %#v", next.Sidebar)
	}
}

func TestHookClientAttachedDoesNotMoveExistingLiveSidebarToNewClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "/dev/old"}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "${@: -1}" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  list-clients) printf '/dev/new\t$1\t@new\t%%new\talpha\n' ;;
  display-message)
    case "$*" in
      *'#{pane_current_path}'*) printf '/tmp/alpha\n' ;;
      *'#{window_id}'*) printf '@new\n' ;;
    esac ;;
  list-panes)
    if [ "$2" = "-a" ]; then
      printf '%%side\t@owner\t0\n'
    fi ;;
esac
`)

	if err := (runtimeRouter{sidebar: runtimeMultiplexer()}).Handle(context.Background(), Route{Path: "hook/client-attached", Flags: map[string]string{"client": "/dev/new", "session": "alpha"}}, nil, nil); err != nil {
		t.Fatalf("hook client-attached error: %v\nlog=%q", err, readLog(t, logPath))
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "join-pane") || strings.Contains(log, "select-pane -t %side") {
		t.Fatalf("client attach should not move an existing live sidebar, log=%q", log)
	}
}

func TestHookClientAttachedRestoresPersistedVisibleSidebarAfterRestart(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "/dev/old", VisibleClients: map[string]bool{"/dev/new": true}}
	state.SessionOrder = []string{"alpha"}
	state.PinnedSessions = []string{"alpha"}
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  printf "can't find session\n" >&2
  exit 1
fi
case "$1" in
  show-options)
    if [ "$2" = "-pv" ]; then
      printf '1\n'
      exit 0
    fi
    case "${@: -1}" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  list-sessions) printf '$1\talpha\t1\t1\n' ;;
  list-clients) printf '/dev/new\t$1\t@1\t%%work\talpha\n' ;;
  display-message)
    case "$*" in
      *'#{pane_current_path}'*) printf '/tmp/alpha\n' ;;
      *'#{window_id}'*) printf '@1\n' ;;
    esac ;;
  list-panes)
    case "$*" in
      *'#{==:#{@session-sidebar-pane},1}'*) ;;
      *'-P'*) ;;
      *) printf '%%work\t0\t0\n' ;;
    esac ;;
  has-session) exit 1 ;;
  new-session) printf '%%side\t@hidden\n' ;;
  set-option) ;;
  join-pane) ;;
  select-pane) ;;
  send-keys) ;;
esac
`)

	if err := (runtimeRouter{sidebar: runtimeMultiplexer()}).Handle(context.Background(), Route{Path: "hook/client-attached", Flags: map[string]string{"client": "/dev/new"}}, nil, nil); err != nil {
		t.Fatalf("hook client-attached error: %v\nlog=%q", err, readLog(t, logPath))
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "select-pane -t %side -R") {
		t.Fatalf("expected persisted open sidebar to be adopted and selected without focus, log=%q", log)
	}
	next, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	if next.Sidebar == nil || !next.Sidebar.Open || next.Sidebar.OwnerClient != "/dev/new" || !next.Sidebar.VisibleClients["/dev/new"] || len(next.Sidebar.VisibleClients) != 1 {
		t.Fatalf("sidebar state = %#v, want open adopted by /dev/new only", next.Sidebar)
	}
	if len(next.PinnedSessions) != 1 || next.PinnedSessions[0] != "alpha" {
		t.Fatalf("pinned sessions changed during adoption: %#v", next.PinnedSessions)
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
      @session-sidebar-width) printf '30\n' ;;
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
