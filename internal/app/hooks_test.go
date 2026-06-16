package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/attention"
)

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
