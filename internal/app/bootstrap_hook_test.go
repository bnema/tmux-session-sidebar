package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRuntimeHooksRegistersResizeCommands(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	// Test-only stub script; it must stay executable so bash resolves it via PATH as tmux.
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	// Safe here: the test sources the repo-local tmux-session-sidebar.tmux script via scriptPath and only calls install_runtime_hooks.
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_runtime_hooks "$2"`, "bash", scriptPath, "/tmp/runtime")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMUX_LOG="+logPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_runtime_hooks error: %v\n%s", err, output)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}

	hooks := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
		fields := strings.Split(line, "\x1f")
		if len(fields) >= 4 && fields[0] == "set-hook" && fields[1] == "-g" {
			hooks[fields[2]] = fields[3]
		}
	}

	clientHook := hooks["client-resized[9704]"]
	if clientHook == "" {
		t.Fatalf("client-resized hook not registered, log=%q", string(content))
	}
	windowHook := hooks["window-resized[9705]"]
	if windowHook == "" {
		t.Fatalf("window-resized hook not registered, log=%q", string(content))
	}

	if !strings.Contains(clientHook, `run-shell -b "/tmp/runtime hook client-resized --client #{q:client_name}"`) {
		t.Fatalf("unexpected client hook %q", clientHook)
	}
	if !strings.Contains(windowHook, `run-shell -b "/tmp/runtime hook window-resized --window #{q:hook_window}"`) {
		t.Fatalf("unexpected window hook %q", windowHook)
	}
}
