package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallResurrectHookInstallsWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\ncase \"$1\" in\n  show-options) ;;\nesac\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_resurrect_hook "$2"`, "bash", scriptPath, "/tmp/with spaces/runtime")
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_resurrect_hook error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if !strings.Contains(log, "set-option\x1f-gq\x1f@resurrect-hook-post-save-layout\x1f/tmp/with spaces/runtime resurrect post-save-layout") {
		t.Fatalf("resurrect hook not installed correctly, log=%q", log)
	}
}

func TestInstallResurrectHookNoopsWhenAlreadyInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\ncase \"$1\" in\n  show-options) printf '/tmp/runtime resurrect post-save-layout\\n' ;;\nesac\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_resurrect_hook "$2"`, "bash", scriptPath, "/tmp/runtime")
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_resurrect_hook error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if strings.Contains(log, "set-option") {
		t.Fatalf("already installed hook should not be overwritten, log=%q", log)
	}
}

func TestInstallResurrectHookPreservesExistingUserHook(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\ncase \"$1\" in\n  show-options) printf 'echo user-hook\\n' ;;\nesac\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_resurrect_hook "$2"`, "bash", scriptPath, "/tmp/runtime")
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_resurrect_hook error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if strings.Contains(log, "set-option") {
		t.Fatalf("existing user hook should not be overwritten, log=%q", log)
	}
}

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

	attachedHook := hooks["client-attached[9701]"]
	if attachedHook == "" {
		t.Fatalf("client-attached hook not registered, log=%q", string(content))
	}
	detachedHook := hooks["client-detached[9702]"]
	if detachedHook == "" {
		t.Fatalf("client-detached hook not registered, log=%q", string(content))
	}
	sessionChangedHook := hooks["client-session-changed[9703]"]
	if sessionChangedHook == "" {
		t.Fatalf("client-session-changed hook not registered, log=%q", string(content))
	}
	clientHook := hooks["client-resized[9704]"]
	if clientHook == "" {
		t.Fatalf("client-resized hook not registered, log=%q", string(content))
	}
	windowHook := hooks["window-resized[9705]"]
	if windowHook == "" {
		t.Fatalf("window-resized hook not registered, log=%q", string(content))
	}

	for name, hook := range map[string]string{
		"client-attached":        attachedHook,
		"client-detached":        detachedHook,
		"client-session-changed": sessionChangedHook,
	} {
		if !strings.Contains(hook, `--client=#{q:hook_client} --session=#{q:hook_session_name}`) {
			t.Fatalf("unexpected %s hook %q", name, hook)
		}
	}
	if !strings.Contains(sessionChangedHook, `run-shell -b "/tmp/runtime hook client-session-changed`) {
		t.Fatalf("unexpected client-session-changed hook %q", sessionChangedHook)
	}
	if !strings.Contains(clientHook, `run-shell -b "/tmp/runtime hook client-resized --client #{q:hook_client}"`) {
		t.Fatalf("unexpected client hook %q", clientHook)
	}
	if !strings.Contains(windowHook, `run-shell -b "/tmp/runtime hook window-resized --window #{q:hook_window}"`) {
		t.Fatalf("unexpected window hook %q", windowHook)
	}
}

func TestMainBootstrapUsesRuntimeDaemonBootstrapWithoutStateDir(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	pluginDir := filepath.Dir(mustAbs(t, filepath.Join("..", "..", "tmux-session-sidebar.tmux")))
	runtimePath := filepath.Join(tmpDir, "runtime")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\ncase \"$1\" in\n  show-options) ;;\nesac\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	ensureRuntime := filepath.Join(pluginDir, "scripts", "ensure-runtime.sh")
	backup, err := os.ReadFile(ensureRuntime)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", ensureRuntime, err)
	}
	defer func() { _ = os.WriteFile(ensureRuntime, backup, 0o755) }()
	if err := os.WriteFile(ensureRuntime, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$TEST_RUNTIME\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", ensureRuntime, err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", filepath.Join(pluginDir, "tmux-session-sidebar.tmux"))
	stateHome := filepath.Join(tmpDir, "xdg-state")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMUX_LOG="+logPath,
		"TEST_RUNTIME="+runtimePath,
		"XDG_STATE_HOME="+stateHome,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap script error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if !strings.Contains(log, "run-shell\x1f-b\x1f"+runtimePath+" daemon bootstrap") {
		t.Fatalf("daemon bootstrap command not registered, log=%q", log)
	}
	if strings.Contains(log, stateHome) || strings.Contains(log, "daemon-control.sh") {
		t.Fatalf("bootstrap leaked shell-computed state dir or daemon-control, log=%q", log)
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q) error: %v", path, err)
	}
	return abs
}
