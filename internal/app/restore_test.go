package app

import (
	"context"
	"strings"
	"testing"

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
