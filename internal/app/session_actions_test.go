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
	tests := []struct {
		name         string
		client       string
		wantContains []string
	}{
		{name: "targets client and carries client to confirmed command", client: "/dev/pts/99", wantContains: []string{"confirm-before -t /dev/pts/99 -p Kill session alpha?", "--client", "/dev/pts/99"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

			err := killSession(context.Background(), map[string]string{"client": tt.client, "session": "alpha"})
			if err != nil {
				t.Fatalf("killSession returned error: %v", err)
			}
			log := readLog(t, logPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(log, want) {
					t.Fatalf("expected log to contain %q, log=%q", want, log)
				}
			}
		})
	}
}

func TestCommandPromptsUseQuotedTmuxInputPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		act  func(context.Context) error
	}{
		{name: "rename prompt", act: func(ctx context.Context) error {
			return renameSession(ctx, map[string]string{"client": "/dev/pts/99", "session": "alpha"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

			if err := tt.act(context.Background()); err != nil {
				t.Fatalf("prompt action error: %v", err)
			}
			log := readLog(t, logPath)
			if !strings.Contains(log, "--name") || !strings.Contains(log, "%%") {
				t.Fatalf("expected quoted tmux input placeholder, log=%q", log)
			}
			if strings.Contains(log, "%%%'") || strings.Contains(log, "%%\\%") {
				t.Fatalf("unexpected escaped placeholder, log=%q", log)
			}
		})
	}
}

func TestCreateAdhocUsesCurrentDirectoryNameWithoutPrompt(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message)
    printf '/tmp/worktree/scratch\n'
    ;;
  list-sessions)
    ;;
esac
`)

	if err := createAdhoc(context.Background(), map[string]string{}); err != nil {
		t.Fatalf("createAdhoc returned error: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "command-prompt") {
		t.Fatalf("expected no ad-hoc prompt, log=%q", log)
	}
	for _, want := range []string{"display-message -p #{pane_current_path}", "new-session -d -s scratch -c /tmp/worktree/scratch", "switch-client -t scratch"} {
		if !strings.Contains(log, want) {
			t.Fatalf("expected log to contain %q, log=%q", want, log)
		}
	}
}

func TestConfirmedKill(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		wantContains []string
	}{
		{
			name: "refreshes sidebar pane",
			script: `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions)
    printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n'
    ;;
  display-message)
    printf '%%1\n'
    ;;
  list-panes)
    printf '%%2\t1\n'
    ;;
esac
`,
			wantContains: []string{"kill-session -t =alpha", "send-keys -t %2 F5"},
		},
		{
			name: "ignores refresh failure after successful kill",
			script: `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions)
    printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n'
    ;;
  kill-session)
    exit 0
    ;;
  *)
    exit 1
    ;;
esac
`,
			wantContains: []string{"kill-session -t =alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := installFakeTmux(t, tt.script)
			err := killSession(context.Background(), map[string]string{"client": "/dev/pts/99", "session": "alpha", "confirmed": "yes"})
			if err != nil {
				t.Fatalf("killSession returned error: %v", err)
			}
			log := readLog(t, logPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(log, want) {
					t.Fatalf("expected log to contain %q, log=%q", want, log)
				}
			}
		})
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	logBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	return string(logBytes)
}
