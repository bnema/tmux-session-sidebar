package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/attention"
)

func TestInstallJSONHooksFailsWithoutClobberingExistingConfigWhenDirectoryCannotCreateTemp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions do not block temp file creation consistently on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	def, ok := agentHookDefNamed("codex")
	if !ok {
		t.Fatal("missing codex hook definition")
	}
	configDir := def.resolvedConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := def.configPath()
	original := []byte("{\n  \"hooks\": {}\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write seed config: %v", err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatalf("chmod config dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })
	if probe, probeErr := os.CreateTemp(configDir, "probe-*"); probeErr == nil {
		probeName := probe.Name()
		_ = probe.Close()
		_ = os.Remove(probeName)
		t.Skip("directory permissions still allow temp file creation in this environment")
	}

	err := installJSONHooks(new(bytes.Buffer), def, true)
	if err == nil {
		t.Fatal("installJSONHooks succeeded with unwritable config directory; want atomic temp-file error")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read config after failed install: %v", readErr)
	}
	if !bytes.Equal(content, original) {
		t.Fatalf("config was clobbered after failed install:\ngot:\n%s\nwant:\n%s", content, original)
	}
}

func TestWriteHookFileAtomicPreservesSymlinkedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real-config.json")
	link := filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("write target config: %v", err)
	}
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Fatalf("symlink config: %v", err)
	}

	if err := writeHookFileAtomic(link, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("writeHookFileAtomic error: %v", err)
	}

	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config link was replaced; mode=%v", linkInfo.Mode())
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target config: %v", err)
	}
	if string(content) != "new\n" {
		t.Fatalf("target config = %q, want new content", content)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target config: %v", err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("target permissions = %v, want 0600", got)
	}
}

func TestWriteHookFileAtomicRejectsDanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "config.json")
	missingTarget := filepath.Join(dir, "missing-config.json")
	if err := os.Symlink(filepath.Base(missingTarget), link); err != nil {
		t.Fatalf("symlink config: %v", err)
	}

	err := writeHookFileAtomic(link, []byte("new\n"), 0o644)
	if err == nil {
		t.Fatal("writeHookFileAtomic error = nil, want dangling symlink error")
	}
	if !strings.Contains(err.Error(), "stat symlink hook config target") {
		t.Fatalf("writeHookFileAtomic error = %v, want stat symlink hook config target", err)
	}
}

func TestInstallJSONHooksPreservesExistingConfigPermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	def, ok := agentHookDefNamed("codex")
	if !ok {
		t.Fatal("missing codex hook definition")
	}
	configDir := def.resolvedConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := def.configPath()
	if err := os.WriteFile(path, []byte("{\n  \"hooks\": {}\n}\n"), 0o600); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	if err := installJSONHooks(new(bytes.Buffer), def, true); err != nil {
		t.Fatalf("installJSONHooks error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %v, want 0600", got)
	}
}

func TestInstallJSONHooksPreservesUserHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	def, ok := agentHookDefNamed("codex")
	if !ok {
		t.Fatal("missing codex hook definition")
	}
	configDir := def.resolvedConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := def.configPath()
	seed := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo user-hook", "timeout": 5}}},
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": def.hookCommand(def.Events[2]), "timeout": 5}}},
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	stdout := new(bytes.Buffer)
	if err := installJSONHooks(stdout, def, true); err != nil {
		t.Fatalf("installJSONHooks error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "echo user-hook") {
		t.Fatalf("user hook removed unexpectedly: %s", text)
	}
	if strings.Count(text, def.ownedMarker()) != len(def.Events) {
		t.Fatalf("owned hooks count = %d, want %d; config=%s", strings.Count(text, def.ownedMarker()), len(def.Events), text)
	}
}

func TestRunHooksSetupSkipsMissingBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	stdout := new(bytes.Buffer)
	if err := runHooksSetup(stdout, map[string]string{"agent": "codex", "yes": "true"}, "", false); err != nil {
		t.Fatalf("runHooksSetup error: %v", err)
	}
	if !strings.Contains(stdout.String(), "skipped (binary not found on PATH)") {
		t.Fatalf("expected missing-binary skip, got: %s", stdout.String())
	}
}

func TestRunHooksSetupInstallsJSONHooksWithoutPreexistingConfigDir(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	def, ok := agentHookDefNamed("codex")
	if !ok {
		t.Fatal("missing codex hook definition")
	}
	if _, err := os.Stat(def.resolvedConfigDir()); !os.IsNotExist(err) {
		t.Fatalf("expected missing config dir, got err=%v", err)
	}

	stdout := new(bytes.Buffer)
	if err := runHooksSetup(stdout, map[string]string{"agent": "codex", "yes": "true"}, "", false); err != nil {
		t.Fatalf("runHooksSetup error: %v", err)
	}
	content, err := os.ReadFile(def.configPath())
	if err != nil {
		t.Fatalf("read installed hooks: %v", err)
	}
	if !strings.Contains(string(content), def.ownedMarker()) {
		t.Fatalf("installed hooks missing marker: %s", string(content))
	}
}

func TestRunHooksSetupInstallsOMPExtension(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(binDir, "omp"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake omp: %v", err)
	}
	def, ok := agentHookDefNamed("omp")
	if !ok {
		t.Fatal("missing omp hook definition")
	}
	if got, want := def.configPath(), filepath.Join(home, ".omp", "agent", "extensions", "cmux-session.ts"); got != want {
		t.Fatalf("omp config path = %q, want %q", got, want)
	}

	stdout := new(bytes.Buffer)
	if err := runHooksSetup(stdout, map[string]string{"agent": "omp", "yes": "true"}, "", false); err != nil {
		t.Fatalf("runHooksSetup error: %v", err)
	}
	content, err := os.ReadFile(def.configPath())
	if err != nil {
		t.Fatalf("read installed omp extension: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		def.ownedMarker(),
		`@oh-my-pi/pi-coding-agent`,
		`pi.on("session_start"`,
		`pi.on("before_agent_start"`,
		`pi.on("agent_end"`,
		`sendHook("stop")`,
		`pi.on("session_shutdown"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed omp extension missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "@earendil-works/pi-coding-agent") {
		t.Fatalf("omp extension imported legacy Pi package:\n%s", text)
	}
}

func TestGeneratedPiStyleExtensionSourcesMatchGoldenOutput(t *testing.T) {
	tests := []struct {
		name string
		def  agentHookDef
		got  string
		want string
	}{
		{
			name: "pi",
			def:  mustAgentHookDef(t, "pi"),
			got:  piExtensionSource(mustAgentHookDef(t, "pi")),
			want: expectedPiExtensionSource(mustAgentHookDef(t, "pi")),
		},
		{
			name: "omp",
			def:  mustAgentHookDef(t, "omp"),
			got:  ompExtensionSource(mustAgentHookDef(t, "omp")),
			want: expectedOMPExtensionSource(mustAgentHookDef(t, "omp")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s generated source changed unexpectedly:\ngot:\n%s\nwant:\n%s", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestOMPExtensionSourceMapsEventsAndDisableEnvExactly(t *testing.T) {
	def := mustAgentHookDef(t, "omp")
	source := ompExtensionSource(def)
	blocks := map[string]string{
		"session_start": `  pi.on("session_start", async () => {
    sendHook("session-start");
  });`,
		"before_agent_start": `  pi.on("before_agent_start", async () => {
    sendHook("prompt-submit");
  });`,
		"agent_end": `  pi.on("agent_end", async () => {
    sendHook("stop");
  });`,
		"session_shutdown": `  pi.on("session_shutdown", async () => {
    sendHook("session-end");
  });`,
	}
	for event, block := range blocks {
		if !strings.Contains(source, block) {
			t.Fatalf("OMP source maps %s with wrong handler block:\nmissing:\n%s\nsource:\n%s", event, block, source)
		}
	}
	if !strings.Contains(source, "process.env.TMUX_SESSION_SIDEBAR_OMP_HOOKS_DISABLED") {
		t.Fatalf("OMP source missing exact disable env var:\n%s", source)
	}
}

func TestOMPInstallIgnoresPICodingAgentDirOverride(t *testing.T) {
	piOverride := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", piOverride)
	t.Setenv("OMP_CODING_AGENT_DIR", "")
	def := mustAgentHookDef(t, "omp")
	wantPath := filepath.Join(home, ".omp", "agent", "extensions", "cmux-session.ts")
	if got := def.configPath(); got != wantPath {
		t.Fatalf("omp config path = %q, want %q", got, wantPath)
	}
	if err := installHooksForAgent(new(bytes.Buffer), def, true); err != nil {
		t.Fatalf("installHooksForAgent error: %v", err)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read OMP extension: %v", err)
	}
	if !strings.Contains(string(content), def.ownedMarker()) {
		t.Fatalf("OMP extension missing marker:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(piOverride, "extensions", "cmux-session.ts")); !os.IsNotExist(err) {
		t.Fatalf("OMP wrote to PI_CODING_AGENT_DIR override, err=%v", err)
	}
}

func TestOMPInstallHonorsOMPCodingAgentDirOverride(t *testing.T) {
	override := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	t.Setenv("OMP_CODING_AGENT_DIR", override)
	def := mustAgentHookDef(t, "omp")
	wantPath := filepath.Join(override, "extensions", "cmux-session.ts")
	if got := def.configPath(); got != wantPath {
		t.Fatalf("omp override config path = %q, want %q", got, wantPath)
	}
	if err := installHooksForAgent(new(bytes.Buffer), def, true); err != nil {
		t.Fatalf("installHooksForAgent error: %v", err)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read override OMP extension: %v", err)
	}
	if !strings.Contains(string(content), def.ownedMarker()) {
		t.Fatalf("override OMP extension missing marker:\n%s", content)
	}
}

func TestUninstallOMPExtensionRemovesMarkedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	def := mustAgentHookDef(t, "omp")
	if err := installHooksForAgent(new(bytes.Buffer), def, true); err != nil {
		t.Fatalf("installHooksForAgent error: %v", err)
	}

	stdout := new(bytes.Buffer)
	if err := uninstallHooksForAgent(stdout, def, true); err != nil {
		t.Fatalf("uninstallHooksForAgent error: %v", err)
	}
	if _, err := os.Stat(def.configPath()); !os.IsNotExist(err) {
		t.Fatalf("OMP extension still exists after uninstall, err=%v", err)
	}
	if !strings.Contains(stdout.String(), "Removed OMP integration") {
		t.Fatalf("uninstall stdout = %q, want removal message", stdout.String())
	}
}

func TestInstallAndUninstallOMPRefuseUnmarkedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	def := mustAgentHookDef(t, "omp")
	path := def.configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	original := []byte("export default function userExtension() {}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write unmarked OMP extension: %v", err)
	}

	if err := installHooksForAgent(new(bytes.Buffer), def, true); err == nil {
		t.Fatal("installHooksForAgent overwrote unmarked OMP extension; want error")
	}
	stdout := new(bytes.Buffer)
	if err := uninstallHooksForAgent(stdout, def, true); err != nil {
		t.Fatalf("uninstallHooksForAgent error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unmarked OMP extension: %v", err)
	}
	if !bytes.Equal(content, original) {
		t.Fatalf("unmarked OMP extension changed:\ngot:\n%s\nwant:\n%s", content, original)
	}
	if !strings.Contains(stdout.String(), "Refusing to remove") {
		t.Fatalf("uninstall stdout = %q, want refusal", stdout.String())
	}
}

func mustAgentHookDef(t *testing.T, name string) agentHookDef {
	t.Helper()
	def, ok := agentHookDefNamed(name)
	if !ok {
		t.Fatalf("missing %s hook definition", name)
	}
	return def
}

func expectedPiExtensionSource(def agentHookDef) string {
	return fmt.Sprintf(`// %s
// Derived from the cmux Pi extension integration design.
// Copyright (c) 2024-present Manaflow, Inc.
// cmux is dual-licensed GPL-3.0-or-later or commercial; see the cmux LICENSE for details.
// This tmux-session-sidebar generated file is an independent implementation inspired by that integration shape.
// DO NOT EDIT MANUALLY. Reinstall with: tmux-session-sidebar hooks %s install

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { spawnSync } from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

function runtimePath(): string {
  const explicit = process.env.TMUX_SESSION_SIDEBAR_BIN;
  if (explicit && explicit.trim()) return explicit.trim();
  const bundled = path.join(os.homedir(), ".tmux", "plugins", "tmux-session-sidebar", ".bin", "tmux-session-sidebar");
  try {
    fs.accessSync(bundled, fs.constants.X_OK);
    return bundled;
  } catch (_) {}
  return "tmux-session-sidebar";
}

function currentPane(): string | null {
  const pane = process.env.TMUX_PANE;
  return pane && pane.trim() ? pane.trim() : null;
}

function sendHook(event: string): void {
  if (process.env.%s === "1") return;
  const pane = currentPane();
  if (!pane) return;
  try {
    spawnSync(runtimePath(), ["hooks", %q, event, "--pane", pane], {
      stdio: "ignore",
      timeout: 5000,
    });
  } catch (_) {}
}

export default function tmuxSessionSidebarPiExtension(pi: ExtensionAPI) {
  pi.on("session_start", async () => {
    sendHook("session-start");
  });
  pi.on("before_agent_start", async () => {
    sendHook("prompt-submit");
  });
  pi.on("agent_end", async () => {
    sendHook("stop");
  });
  pi.on("session_shutdown", async () => {
    sendHook("session-end");
  });
}
`, def.ownedMarker(), def.Name, def.DisableEnvVar, def.Name)
}

func expectedOMPExtensionSource(def agentHookDef) string {
	return fmt.Sprintf(`// %s
// Generated OMP extension for tmux-session-sidebar agent attention.
// DO NOT EDIT MANUALLY. Reinstall with: tmux-session-sidebar hooks %s install

import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { spawnSync } from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

function runtimePath(): string {
  const explicit = process.env.TMUX_SESSION_SIDEBAR_BIN;
  if (explicit && explicit.trim()) return explicit.trim();
  const bundled = path.join(os.homedir(), ".tmux", "plugins", "tmux-session-sidebar", ".bin", "tmux-session-sidebar");
  try {
    fs.accessSync(bundled, fs.constants.X_OK);
    return bundled;
  } catch (_) {}
  return "tmux-session-sidebar";
}

function currentPane(): string | null {
  const pane = process.env.TMUX_PANE;
  return pane && pane.trim() ? pane.trim() : null;
}

function sendHook(event: string): void {
  if (process.env.%s === "1") return;
  const pane = currentPane();
  if (!pane) return;
  try {
    spawnSync(runtimePath(), ["hooks", %q, event, "--pane", pane], {
      stdio: "ignore",
      timeout: 5000,
    });
  } catch (_) {}
}

export default function tmuxSessionSidebarOMPExtension(pi: ExtensionAPI) {
  pi.on("session_start", async () => {
    sendHook("session-start");
  });
  pi.on("before_agent_start", async () => {
    sendHook("prompt-submit");
  });
  pi.on("agent_end", async () => {
    sendHook("stop");
  });
  pi.on("session_shutdown", async () => {
    sendHook("session-end");
  });
}
`, def.ownedMarker(), def.Name, def.DisableEnvVar, def.Name)
}

func TestRunHooksCommandRoutesOMPStopEventFromTMUXPane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%2")
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\t$1\n' ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)

	route := Route{Path: "hooks/run", Args: []string{"hooks", "omp", "stop"}, Flags: map[string]string{}}
	if err := (runtimeRouter{}).Handle(context.Background(), route, new(bytes.Buffer), new(bytes.Buffer)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	decoded := attention.DecodeStateMap(state.AgentAttention)
	if !decoded["$1"].Attention {
		t.Fatalf("attention = false, want true; state=%#v", decoded["$1"])
	}
	if decoded["$1"].Panes["%2"].Agent != "omp" {
		t.Fatalf("agent = %q, want omp", decoded["$1"].Panes["%2"].Agent)
	}
}

func TestRunHooksCommandRoutesAgentEventFromTMUXPane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%2")
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\t$1\n' ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)

	route := Route{Path: "hooks/run", Args: []string{"hooks", "codex", "stop"}, Flags: map[string]string{}}
	if err := (runtimeRouter{}).Handle(context.Background(), route, new(bytes.Buffer), new(bytes.Buffer)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	state, err := loadSidebarState(context.Background())
	if err != nil {
		t.Fatalf("loadSidebarState error: %v", err)
	}
	decoded := attention.DecodeStateMap(state.AgentAttention)
	if !decoded["$1"].Attention {
		t.Fatalf("attention = false, want true; state=%#v", decoded["$1"])
	}
	if decoded["$1"].Panes["%2"].Agent != "codex" {
		t.Fatalf("agent = %q, want codex", decoded["$1"].Panes["%2"].Agent)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("expected agent hook event to refresh open sidebars, log=%q", log)
	}
}

func TestRunHooksCommandDoesNotRefreshSidebarsForSuppressedAttentionEvent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%2")
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\t$1\n' ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) printf '/dev/pts/1\t$1\t@1\t%%2\t/dev/pts/1\n' ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)
	store := sessionOrderStore()
	state, err := store.Load(context.Background(), "tmux")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.AgentAttention = attention.EncodeStateMap(map[string]attention.State{
		"$1": {CurrentAcknowledged: true, LastVisitedAt: time.Now().UTC(), Panes: map[string]attention.PaneState{"%2": {Agent: "codex"}}},
	})
	if err := store.Save(context.Background(), "tmux", state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	route := Route{Path: "hooks/run", Args: []string{"hooks", "codex", "stop"}, Flags: map[string]string{}}
	if err := (runtimeRouter{}).Handle(context.Background(), route, new(bytes.Buffer), new(bytes.Buffer)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if log := readLog(t, logPath); strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("suppressed attention hook should not refresh sidebars, log=%q", log)
	}
}

func TestRunHooksCommandDoesNotRefreshSidebarsForRunningEvent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%2")
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  show-options)
    case "$3" in
      @session-sidebar-key) printf 'M-b\n' ;;
      @session-sidebar-width) printf '30\n' ;;
      @session-sidebar-project-roots) printf '\n' ;;
      @session-sidebar-close-after-switch) printf 'off\n' ;;
      @session-sidebar-heat-colors) printf 'on\n' ;;
      @session-sidebar-heat-half-life-hours) printf '8\n' ;;
      @session-sidebar-heat-stale-hours) printf '24\n' ;;
      @session-sidebar-heat-refresh-seconds) printf '5\n' ;;
      @session-sidebar-activity-debug-log) printf 'off\n' ;;
      @session-sidebar-agent-attention) printf 'on\n' ;;
      *) printf '\n' ;;
    esac ;;
  display-message) printf 'alpha\t$1\n' ;;
  list-sessions) printf '$1\talpha\t1\t0\n' ;;
  list-clients) ;;
  list-panes) printf '%%sidebar\n' ;;
esac
`)

	route := Route{Path: "hooks/run", Args: []string{"hooks", "codex", "prompt-submit"}, Flags: map[string]string{}}
	if err := (runtimeRouter{}).Handle(context.Background(), route, new(bytes.Buffer), new(bytes.Buffer)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if log := readLog(t, logPath); strings.Contains(log, "send-keys -t %sidebar F5") {
		t.Fatalf("running hook should not refresh sidebars, log=%q", log)
	}
}

func TestGeneratedHookSourcesCarryCMUXCredits(t *testing.T) {
	defs := []string{"opencode", "pi", "amp"}
	for _, name := range defs {
		def, ok := agentHookDefNamed(name)
		if !ok {
			t.Fatalf("missing def %s", name)
		}
		var source string
		switch name {
		case "opencode":
			source = openCodePluginSource(def)
		case "pi":
			source = piExtensionSource(def)
		case "amp":
			source = ampPluginSource(def)
		}
		if !strings.Contains(source, "Derived from the cmux") {
			t.Fatalf("missing cmux credit for %s", name)
		}
		if !strings.Contains(source, "GPL-3.0-or-later") {
			t.Fatalf("missing license mention for %s", name)
		}
		if !strings.Contains(source, def.ownedMarker()) {
			t.Fatalf("missing ownership marker for %s", name)
		}
	}
}
