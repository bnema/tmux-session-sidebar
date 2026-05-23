package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

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
      @session-sidebar-heat-refresh-seconds) printf '300\n' ;;
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

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
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

	cfg := ports.ConfigSnapshot{HeatHalfLifeHours: 8, HeatStaleHours: 24, AttentionQuietSeconds: 120, ActivityDebugLog: true}
	if err := captureLiveSidebarHeat(context.Background(), cfg); err != nil {
		t.Fatalf("captureLiveSidebarHeat error: %v", err)
	}

	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "tmux-session-sidebar", "activity.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read activity log: %v", err)
	}
	if !strings.Contains(string(content), "status=activity-detected") {
		t.Fatalf("activity log missing transition status: %q", string(content))
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
