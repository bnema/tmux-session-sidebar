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
	layoutChangedHook := hooks["window-layout-changed[9706]"]
	if layoutChangedHook == "" {
		t.Fatalf("window-layout-changed hook not registered, log=%q", string(content))
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
	if !strings.Contains(layoutChangedHook, `run-shell -b "/tmp/runtime hook window-layout-changed --window #{q:hook_window}"`) {
		t.Fatalf("unexpected window-layout-changed hook %q", layoutChangedHook)
	}
}

func TestInstallSidebarWheelBindingsWrapsDefaultWheelBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	fakeScript := `#!/usr/bin/env bash
printf '%s\x1f' "$@" >> "$TMUX_LOG"
printf '\n' >> "$TMUX_LOG"
if [ "$1" = list-keys ]; then
  printf 'WheelUpPane if-shell -F "#{||:#{alternate_on},#{pane_in_mode},#{mouse_any_flag}}" { send-keys -M } { copy-mode -e }\n'
fi
`
	if err := os.WriteFile(fakeTmux, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_sidebar_wheel_bindings`, "bash", scriptPath)
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_sidebar_wheel_bindings error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	wheelUp := bindingLogLine(t, log, "WheelUpPane")
	wheelDown := bindingLogLine(t, log, "WheelDownPane")
	for key, line := range map[string]string{"WheelUpPane": wheelUp, "WheelDownPane": wheelDown} {
		if !strings.Contains(line, "if-shell\x1f-F\x1f-t\x1f=\x1f#{==:#{@session-sidebar-pane},1}") {
			t.Fatalf("%s guard should evaluate against mouse target pane, line=%q log=%q", key, line, log)
		}
		if !strings.Contains(line, "select-pane -t =") || !strings.Contains(line, "send-keys -M") {
			t.Fatalf("%s should focus mouse pane and forward event, line=%q log=%q", key, line, log)
		}
	}
	if !strings.Contains(wheelUp, "copy-mode -e") {
		t.Fatalf("WheelUpPane binding should preserve default copy-mode fallback, line=%q log=%q", wheelUp, log)
	}
	if !strings.Contains(wheelDown, `if-shell -F "#{mouse_any_flag}" "send-keys -M"`) {
		t.Fatalf("WheelDownPane binding should preserve mouse forwarding fallback, line=%q log=%q", wheelDown, log)
	}
}

func TestInstallSidebarWheelBindingsPreservesCustomUserBinding(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	fakeScript := `#!/usr/bin/env bash
printf '%s\x1f' "$@" >> "$TMUX_LOG"
printf '\n' >> "$TMUX_LOG"
if [ "$1" = list-keys ]; then
  printf 'WheelUpPane display-message user-wheel-up\n'
  printf 'WheelDownPane display-message user-wheel-down\n'
fi
`
	if err := os.WriteFile(fakeTmux, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_sidebar_wheel_bindings`, "bash", scriptPath)
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_sidebar_wheel_bindings error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if strings.Contains(log, "bind-key\x1f-T\x1froot\x1fWheelUpPane") || strings.Contains(log, "bind-key\x1f-T\x1froot\x1fWheelDownPane") {
		t.Fatalf("custom wheel bindings should not be overwritten, log=%q", log)
	}
}

func TestInstallSidebarWheelBindingsIgnoresOtherBindingsMentioningWheelKeys(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	fakeScript := `#!/usr/bin/env bash
printf '%s\x1f' "$@" >> "$TMUX_LOG"
printf '\n' >> "$TMUX_LOG"
if [ "$1" = list-keys ]; then
  printf 'bind-key  -T root M-x display-message WheelUpPane\n'
fi
`
	if err := os.WriteFile(fakeTmux, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "tmux-session-sidebar.tmux"))
	if err != nil {
		t.Fatalf("Abs script path error: %v", err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `source "$1"; install_sidebar_wheel_bindings`, "bash", scriptPath)
	cmd.Env = append(os.Environ(), "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMUX_LOG="+logPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install_sidebar_wheel_bindings error: %v\n%s", err, output)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", logPath, err)
	}
	log := string(content)
	if !strings.Contains(log, "bind-key\x1f-T\x1froot\x1fWheelUpPane") {
		t.Fatalf("unrelated binding mentioning WheelUpPane should not block sidebar wheel binding, log=%q", log)
	}
}

func TestMainBootstrapUsesRuntimeDaemonBootstrapWithoutStateDir(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	pluginDir := filepath.Dir(mustAbs(t, filepath.Join("..", "..", "tmux-session-sidebar.tmux")))
	testPluginDir := filepath.Join(tmpDir, "plugin")
	if err := os.MkdirAll(filepath.Join(testPluginDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(test plugin scripts) error: %v", err)
	}
	copyFile(t, filepath.Join(pluginDir, "tmux-session-sidebar.tmux"), filepath.Join(testPluginDir, "tmux-session-sidebar.tmux"), 0o755)
	copyFile(t, filepath.Join(pluginDir, "scripts", "remove-git-update-hook.sh"), filepath.Join(testPluginDir, "scripts", "remove-git-update-hook.sh"), 0o755)
	runtimePath := filepath.Join(tmpDir, "runtime")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf '%s\\x1f' \"$@\" >> \"$TMUX_LOG\"\nprintf '\\n' >> \"$TMUX_LOG\"\ncase \"$1\" in\n  show-options) ;;\nesac\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeTmux, err)
	}
	ensureRuntime := filepath.Join(testPluginDir, "scripts", "ensure-runtime.sh")
	if err := os.WriteFile(ensureRuntime, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$TEST_RUNTIME\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", ensureRuntime, err)
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", filepath.Join(testPluginDir, "tmux-session-sidebar.tmux"))
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
	if !strings.Contains(log, "bind-key\x1f-T\x1froot\x1fWheelUpPane") || !strings.Contains(log, "bind-key\x1f-T\x1froot\x1fWheelDownPane") {
		t.Fatalf("bootstrap did not install sidebar wheel focus bindings, log=%q", log)
	}
	if strings.Contains(log, stateHome) || strings.Contains(log, "daemon-control.sh") {
		t.Fatalf("bootstrap leaked shell-computed state dir or daemon-control, log=%q", log)
	}
}

func bindingLogLine(t *testing.T, log string, key string) string {
	t.Helper()
	prefix := "bind-key\x1f-T\x1froot\x1f" + key + "\x1f"
	for line := range strings.SplitSeq(strings.TrimSpace(log), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("%s binding not installed, log=%q", key, log)
	return ""
}

func copyFile(t *testing.T, src string, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", dst, err)
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
