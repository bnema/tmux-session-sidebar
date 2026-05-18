package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeTmux(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(dir, "tmux.log")
	t.Setenv("TMUX_LOG", logPath)
	return logPath
}

func TestKillSessionConfirmationTargetsClient(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

	err := killSession(context.Background(), map[string]string{"client": "/dev/pts/99", "session": "alpha"})
	if err != nil {
		t.Fatalf("killSession returned error: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "confirm-before -t /dev/pts/99 -p Kill session alpha?") {
		t.Fatalf("expected confirm-before to target the sidebar client, log=%q", log)
	}
	if !strings.Contains(log, "--client") || !strings.Contains(log, "/dev/pts/99") {
		t.Fatalf("expected confirmed kill command to carry client for refresh, log=%q", log)
	}
}

func TestCommandPromptsUseQuotedTmuxInputPlaceholder(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

	if err := createAdhoc(context.Background(), map[string]string{"client": "/dev/pts/99"}); err != nil {
		t.Fatalf("createAdhoc prompt error: %v", err)
	}
	if err := renameSession(context.Background(), map[string]string{"client": "/dev/pts/99", "session": "alpha"}); err != nil {
		t.Fatalf("renameSession prompt error: %v", err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "--name") || !strings.Contains(log, "%%") {
		t.Fatalf("expected quoted tmux input placeholder, log=%q", log)
	}
	if strings.Contains(log, "%%%") {
		t.Fatalf("unexpected escaped placeholder, log=%q", log)
	}
}

func TestConfirmedKillRefreshesSidebarPane(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message)
    printf '%%1\n'
    ;;
  list-panes)
    printf '%%2\t1\n'
    ;;
esac
`)

	err := killSession(context.Background(), map[string]string{"client": "/dev/pts/99", "session": "alpha", "confirmed": "yes"})
	if err != nil {
		t.Fatalf("killSession returned error: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "kill-session -t =alpha") {
		t.Fatalf("expected session to be killed by exact target, log=%q", log)
	}
	if !strings.Contains(log, "send-keys -t %2 F5") {
		t.Fatalf("expected successful kill to refresh sidebar pane, log=%q", log)
	}
}

func TestConfirmedKillIgnoresRefreshFailureAfterSuccessfulKill(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  kill-session)
    exit 0
    ;;
  *)
    exit 1
    ;;
esac
`)

	err := killSession(context.Background(), map[string]string{"client": "/dev/pts/99", "session": "alpha", "confirmed": "yes"})
	if err != nil {
		t.Fatalf("killSession returned error after successful kill with failed refresh: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "kill-session -t =alpha") {
		t.Fatalf("expected session to be killed by exact target, log=%q", log)
	}
}
