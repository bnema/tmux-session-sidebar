package tmuxcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	processadapter "github.com/bnema/tmux-session-sidebar/adapters/process"
)

func TestAttachSingletonSidebarReopensBesideBottomFocusedStackedPanes(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := fmt.Sprintf("tss-test-%d", os.Getpid())
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "100", "-y", "30")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "20", "-y", "30")
	sidebarPane := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", singletonSidebarSessionName+":", "#{pane_id}"))
	runTmux(t, ctx, realTmux, socketName, "set-option", "-p", "-t", sidebarPane, optionSidebarPane, "1")

	client := Client{Process: processadapter.Runner{}}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}

	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_width}\t#{pane_height}")
	assertSidebarIsFullHeightLeftColumn(t, rows, sidebarPane)
}

func installTmuxSocketWrapper(t *testing.T, realTmux string, socketName string) {
	t.Helper()
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "tmux")
	content := fmt.Sprintf("#!/usr/bin/env bash\nexec %q -f /dev/null -L %q \"$@\"\n", realTmux, socketName)
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		t.Fatalf("write tmux wrapper: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runTmux(t *testing.T, ctx context.Context, realTmux string, socketName string, args ...string) {
	t.Helper()
	_ = runTmuxOutput(t, ctx, realTmux, socketName, args...)
}

func runTmuxOutput(t *testing.T, ctx context.Context, realTmux string, socketName string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-f", "/dev/null", "-L", socketName}, args...)
	output, err := exec.CommandContext(ctx, realTmux, cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

func assertSidebarIsFullHeightLeftColumn(t *testing.T, rows string, sidebarPane string) {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			t.Fatalf("malformed list-panes row %q", line)
		}
		if fields[0] == sidebarPane {
			if fields[1] != "0" || fields[2] != "20" || fields[3] != "30" {
				t.Fatalf("sidebar pane geometry = left %s width %s height %s, want full-height left column 0/20/30\nall panes:\n%s", fields[1], fields[2], fields[3], rows)
			}
			return
		}
	}
	t.Fatalf("sidebar pane %s not found in panes:\n%s", sidebarPane, rows)
}
