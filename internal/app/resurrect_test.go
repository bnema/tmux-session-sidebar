package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResurrectPostSaveLayoutRemovesInternalSessionAndVisibleSidebarPane(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tmux_resurrect.txt")
	content := strings.Join([]string{
		"pane\talpha\t0\t1\t:*\t0\twork\t:/tmp/alpha\t1\tvim\t:vim",
		"pane\talpha\t0\t1\t:*\t1\tsidebar\t:/tmp/sidebar\t0\ttmux-session-sidebar\t:",
		"window\talpha\t0\t:editor\t1\t:*\twith-sidebar-layout\ton",
		"pane\t__tmux-session-sidebar\t0\t1\t:*\t0\tsidebar\t:/tmp/sidebar\t1\ttmux-session-sidebar\t:",
		"window\t__tmux-session-sidebar\t0\t:sidebar\t1\t:*\tparked-layout\ton",
		"grouped_session\tmonitor\t__tmux-session-sidebar\t:0\t:0",
		"state\talpha\talpha",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-panes)
    printf 'alpha\t0\t1\t1\t@alpha0\n'
    ;;
  show-options)
    if [ "$6" = '@session-sidebar-window-layout' ]; then
      printf 'without-sidebar-layout\n'
    fi
    ;;
esac
`)

	if err := resurrectPostSaveLayout(context.Background(), file); err != nil {
		t.Fatalf("resurrectPostSaveLayout() error = %v", err)
	}

	gotBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read sanitized file: %v", err)
	}
	got := string(gotBytes)
	if strings.Contains(got, "__tmux-session-sidebar") {
		t.Fatalf("sanitized file still contains internal session reference: %q", got)
	}
	if strings.Contains(got, "\t1\tsidebar\t:/tmp/sidebar") {
		t.Fatalf("sanitized file still contains visible sidebar pane: %q", got)
	}
	if !strings.Contains(got, "window\talpha\t0\t:editor\t1\t:*\twithout-sidebar-layout\ton") {
		t.Fatalf("window layout was not replaced with hidden layout: %q", got)
	}
	if !strings.Contains(got, "pane\talpha\t0\t1\t:*\t0\twork") {
		t.Fatalf("user pane was removed unexpectedly: %q", got)
	}
}

func TestResurrectPostSaveLayoutPreservesExistingFileMode(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tmux_resurrect.txt")
	if err := os.WriteFile(file, []byte("pane\t__tmux-session-sidebar\t0\t1\t:*\t0\tsidebar\t:/tmp\t1\ttmux-session-sidebar\t:\n"), 0o640); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-panes) ;;
esac
`)

	if err := resurrectPostSaveLayout(context.Background(), file); err != nil {
		t.Fatalf("resurrectPostSaveLayout() error = %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("stat sanitized file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("file mode = %v, want 0640", got)
	}
}

func TestResurrectPostSaveLayoutKeepsUserLayoutWhenHiddenLayoutMissing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tmux_resurrect.txt")
	content := "pane\talpha\t0\t1\t:*\t1\tsidebar\t:/tmp/sidebar\t0\ttmux-session-sidebar\t:\nwindow\talpha\t0\t:editor\t1\t:*\twith-sidebar-layout\ton\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-panes) printf 'alpha\t0\t1\t1\t@alpha0\n' ;;
  show-options) ;;
esac
`)

	if err := resurrectPostSaveLayout(context.Background(), file); err != nil {
		t.Fatalf("resurrectPostSaveLayout() error = %v", err)
	}
	gotBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read sanitized file: %v", err)
	}
	got := string(gotBytes)
	if strings.Contains(got, "sidebar\t:/tmp/sidebar") {
		t.Fatalf("sidebar pane was not removed: %q", got)
	}
	if !strings.Contains(got, "with-sidebar-layout") {
		t.Fatalf("layout should stay unchanged when hidden layout is missing: %q", got)
	}
}

func TestResurrectCommandDispatchesPostSaveLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with spaces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	file := filepath.Join(dir, "tmux_resurrect.txt")
	if err := os.WriteFile(file, []byte("pane\t__tmux-session-sidebar\t0\t1\t:*\t0\tsidebar\t:/tmp\t1\ttmux-session-sidebar\t:\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-panes) ;;
esac
`)

	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "resurrect/post-save-layout", Args: []string{file}}, nil, nil); err != nil {
		t.Fatalf("Handle resurrect/post-save-layout error = %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read sanitized file: %v", err)
	}
	if strings.TrimSpace(string(got)) != "" {
		t.Fatalf("internal session line not removed: %q", string(got))
	}
}
