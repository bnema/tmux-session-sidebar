package app

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestWithSidebarFollowMovesOpenSidebarWhenConfiguredToStayOpen(t *testing.T) {
	var calls [][]string
	currentWindow := "@old"
	restoreConfig := stubLoadTmuxConfig(t, ports.ConfigSnapshot{CloseAfterSwitch: false})
	defer restoreConfig()
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "tmux" {
			t.Fatalf("command name = %q, want tmux", name)
		}
		calls = append(calls, append([]string(nil), args...))
		joined := strings.Join(args, "\x00")
		if joined == "switch-client\x00-c\x00client-1\x00-t\x00beta" {
			currentWindow = "@new"
			return "", nil
		}
		switch joined {
		case "display-message\x00-p\x00-t\x00client-1\x00#{window_id}":
			return currentWindow + "\n", nil
		case "list-panes\x00-t\x00@old\x00-F\x00#{pane_id}\t#{@session-sidebar-pane}":
			return "%9\t1\n", nil
		case "show-options\x00-gvq\x00@session-sidebar-width":
			return "20\n", nil
		case "display-message\x00-p\x00-t\x00client-1\x00#{pane_current_path}":
			return "/tmp\n", nil
		case "split-window\x00-P\x00-F\x00#{pane_id}\x00-t\x00@new\x00-hbf\x00-l\x0020\x00-c\x00/tmp\x00" + testExecutable(t) + "\x00ui\x00run\x00--client\x00client-1":
			return "%10\n", nil
		case "set-option\x00-p\x00-t\x00%10\x00@session-sidebar-pane\x001":
			return "", nil
		case "kill-pane\x00-t\x00%9":
			return "", nil
		default:
			return "", nil
		}
	})
	defer restore()

	err := withSidebarFollow(context.Background(), "client-1", func() error {
		_, _ = tmux(context.Background(), "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	wantSubsequence := [][]string{
		{"switch-client", "-c", "client-1", "-t", "beta"},
		{"split-window", "-P", "-F", "#{pane_id}", "-t", "@new", "-hbf", "-l", "20", "-c", "/tmp", testExecutable(t), "ui", "run", "--client", "client-1"},
		{"kill-pane", "-t", "%9"},
	}
	assertCallSubsequence(t, calls, wantSubsequence)
}

func TestWithSidebarFollowClosesOpenSidebarWhenConfiguredCloseAfterSwitch(t *testing.T) {
	var calls [][]string
	restoreConfig := stubLoadTmuxConfig(t, ports.ConfigSnapshot{CloseAfterSwitch: true})
	defer restoreConfig()
	restore := stubCommandRunner(t, func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		switch strings.Join(args, "\x00") {
		case "display-message\x00-p\x00-t\x00client-1\x00#{window_id}":
			return "@old\n", nil
		case "list-panes\x00-t\x00@old\x00-F\x00#{pane_id}\t#{@session-sidebar-pane}":
			return "%9\t1\n", nil
		case "kill-pane\x00-t\x00%9":
			return "", nil
		default:
			return "", nil
		}
	})
	defer restore()

	err := withSidebarFollow(context.Background(), "client-1", func() error {
		_, _ = tmux(context.Background(), "switch-client", "-c", "client-1", "-t", "beta")
		return nil
	})
	if err != nil {
		t.Fatalf("withSidebarFollow returned error: %v", err)
	}

	assertCallSubsequence(t, calls, [][]string{
		{"switch-client", "-c", "client-1", "-t", "beta"},
		{"kill-pane", "-t", "%9"},
	})
	for _, call := range calls {
		if len(call) > 0 && call[0] == "split-window" {
			t.Fatalf("split-window called despite close-after-switch=on: %#v", calls)
		}
	}
}

func stubCommandRunner(t *testing.T, runner func(context.Context, string, ...string) (string, error)) func() {
	t.Helper()
	old := commandRunner
	commandRunner = runner
	return func() { commandRunner = old }
}

func stubLoadTmuxConfig(t *testing.T, config ports.ConfigSnapshot) func() {
	t.Helper()
	old := loadTmuxConfig
	loadTmuxConfig = func(context.Context) (ports.ConfigSnapshot, error) { return config, nil }
	return func() { loadTmuxConfig = old }
}

func assertCallSubsequence(t *testing.T, calls [][]string, want [][]string) {
	t.Helper()
	at := 0
	for _, call := range calls {
		if at < len(want) && reflect.DeepEqual(call, want[at]) {
			at++
		}
	}
	if at != len(want) {
		t.Fatalf("calls missing subsequence\nwant: %#v\ngot:  %#v", want, calls)
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}
