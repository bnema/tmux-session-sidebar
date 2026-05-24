package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type agentHookFormat string

const (
	agentHookFormatFlatJSON       agentHookFormat = "flat-json"
	agentHookFormatNestedJSON     agentHookFormat = "nested-json"
	agentHookFormatOpenCodePlugin agentHookFormat = "opencode-plugin"
	agentHookFormatPiExtension    agentHookFormat = "pi-extension"
	agentHookFormatAmpPlugin      agentHookFormat = "amp-plugin"
	agentHookFormatRovoYAML       agentHookFormat = "rovo-yaml"
)

type agentHookEvent struct {
	AgentEvent   string
	SidebarEvent string
}

type agentHookDef struct {
	Name                       string
	DisplayName                string
	BinaryName                 string
	ConfigDir                  string
	ConfigFile                 string
	ConfigDirEnvOverride       string
	ConfigDirEnvOverrideSubdir string
	CreateConfigDirIfMissing   bool
	DisableEnvVar              string
	Format                     agentHookFormat
	Events                     []agentHookEvent
	Aliases                    []string
}

func supportedAgentHookDefs() []agentHookDef {
	return []agentHookDef{
		{
			Name:                 "codex",
			DisplayName:          "Codex",
			BinaryName:           "codex",
			ConfigDir:            ".codex",
			ConfigFile:           "hooks.json",
			ConfigDirEnvOverride: "CODEX_HOME",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_CODEX_HOOKS_DISABLED",
			Format:               agentHookFormatNestedJSON,
			Events:               []agentHookEvent{{"SessionStart", "session-start"}, {"UserPromptSubmit", "prompt-submit"}, {"Stop", "stop"}},
		},
		{
			Name:                       "grok",
			DisplayName:                "Grok",
			BinaryName:                 "grok",
			ConfigDir:                  ".grok",
			ConfigDirEnvOverride:       "GROK_HOME",
			ConfigDirEnvOverrideSubdir: "hooks",
			ConfigFile:                 "cmux-session.json",
			CreateConfigDirIfMissing:   true,
			DisableEnvVar:              "TMUX_SESSION_SIDEBAR_GROK_HOOKS_DISABLED",
			Format:                     agentHookFormatNestedJSON,
			Events:                     []agentHookEvent{{"SessionStart", "session-start"}, {"UserPromptSubmit", "prompt-submit"}, {"Stop", "stop"}, {"Notification", "notification"}, {"SessionEnd", "session-end"}},
		},
		{
			Name:                 "opencode",
			DisplayName:          "OpenCode",
			BinaryName:           "opencode",
			ConfigDir:            ".config/opencode",
			ConfigFile:           "plugins/cmux-session.js",
			ConfigDirEnvOverride: "OPENCODE_CONFIG_DIR",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_OPENCODE_HOOKS_DISABLED",
			Format:               agentHookFormatOpenCodePlugin,
		},
		{
			Name:                 "pi",
			DisplayName:          "Pi",
			BinaryName:           "pi",
			ConfigDir:            ".pi/agent",
			ConfigFile:           "extensions/cmux-session.ts",
			ConfigDirEnvOverride: "PI_CODING_AGENT_DIR",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_PI_HOOKS_DISABLED",
			Format:               agentHookFormatPiExtension,
		},
		{
			Name:          "amp",
			DisplayName:   "Amp",
			BinaryName:    "amp",
			ConfigDir:     ".config/amp",
			ConfigFile:    "plugins/cmux-session.ts",
			DisableEnvVar: "TMUX_SESSION_SIDEBAR_AMP_HOOKS_DISABLED",
			Format:        agentHookFormatAmpPlugin,
		},
		{
			Name:          "cursor",
			DisplayName:   "Cursor CLI",
			BinaryName:    "cursor-agent",
			ConfigDir:     ".cursor",
			ConfigFile:    "hooks.json",
			DisableEnvVar: "TMUX_SESSION_SIDEBAR_CURSOR_HOOKS_DISABLED",
			Format:        agentHookFormatFlatJSON,
			Events:        []agentHookEvent{{"beforeSubmitPrompt", "prompt-submit"}, {"stop", "stop"}, {"afterAgentResponse", "agent-response"}, {"beforeShellExecution", "shell-exec"}, {"afterShellExecution", "shell-done"}},
		},
		{
			Name:          "gemini",
			DisplayName:   "Gemini",
			BinaryName:    "gemini",
			ConfigDir:     ".gemini",
			ConfigFile:    "settings.json",
			DisableEnvVar: "TMUX_SESSION_SIDEBAR_GEMINI_HOOKS_DISABLED",
			Format:        agentHookFormatNestedJSON,
			Events:        []agentHookEvent{{"SessionStart", "session-start"}, {"BeforeAgent", "prompt-submit"}, {"AfterAgent", "stop"}, {"SessionEnd", "session-end"}},
		},
		{
			Name:          "rovodev",
			DisplayName:   "Rovo Dev",
			BinaryName:    "acli",
			ConfigDir:     ".rovodev",
			ConfigFile:    "config.yml",
			DisableEnvVar: "TMUX_SESSION_SIDEBAR_ROVODEV_HOOKS_DISABLED",
			Format:        agentHookFormatRovoYAML,
			Events:        []agentHookEvent{{"on_complete", "stop"}, {"on_error", "stop"}, {"on_tool_permission", "notification"}},
			Aliases:       []string{"rovo"},
		},
		{
			Name:                 "copilot",
			DisplayName:          "Copilot",
			BinaryName:           "copilot",
			ConfigDir:            ".copilot",
			ConfigFile:           "config.json",
			ConfigDirEnvOverride: "COPILOT_HOME",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_COPILOT_HOOKS_DISABLED",
			Format:               agentHookFormatNestedJSON,
			Events:               []agentHookEvent{{"SessionStart", "session-start"}, {"Stop", "stop"}, {"Notification", "notification"}, {"SessionEnd", "session-end"}},
		},
		{
			Name:                 "codebuddy",
			DisplayName:          "CodeBuddy",
			BinaryName:           "codebuddy",
			ConfigDir:            ".codebuddy",
			ConfigFile:           "settings.json",
			ConfigDirEnvOverride: "CODEBUDDY_CONFIG_DIR",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_CODEBUDDY_HOOKS_DISABLED",
			Format:               agentHookFormatNestedJSON,
			Events:               []agentHookEvent{{"SessionStart", "session-start"}, {"Stop", "stop"}, {"Notification", "notification"}, {"SessionEnd", "session-end"}},
		},
		{
			Name:          "factory",
			DisplayName:   "Factory",
			BinaryName:    "droid",
			ConfigDir:     ".factory",
			ConfigFile:    "settings.json",
			DisableEnvVar: "TMUX_SESSION_SIDEBAR_FACTORY_HOOKS_DISABLED",
			Format:        agentHookFormatNestedJSON,
			Events:        []agentHookEvent{{"SessionStart", "session-start"}, {"Stop", "stop"}, {"Notification", "notification"}, {"SessionEnd", "session-end"}},
		},
		{
			Name:                 "qoder",
			DisplayName:          "Qoder",
			BinaryName:           "qodercli",
			ConfigDir:            ".qoder",
			ConfigFile:           "settings.json",
			ConfigDirEnvOverride: "QODER_CONFIG_DIR",
			DisableEnvVar:        "TMUX_SESSION_SIDEBAR_QODER_HOOKS_DISABLED",
			Format:               agentHookFormatNestedJSON,
			Events:               []agentHookEvent{{"SessionStart", "session-start"}, {"Stop", "stop"}, {"SessionEnd", "session-end"}},
		},
	}
}

func agentHookDefNamed(name string) (agentHookDef, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, def := range supportedAgentHookDefs() {
		if def.Name == normalized || slices.Contains(def.Aliases, normalized) {
			return def, true
		}
	}
	return agentHookDef{}, false
}

func (d agentHookDef) resolvedConfigDir() string {
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		if dir, err := os.UserHomeDir(); err == nil {
			home = dir
		}
	}
	if d.ConfigDirEnvOverride != "" {
		if raw := strings.TrimSpace(os.Getenv(d.ConfigDirEnvOverride)); raw != "" {
			if d.ConfigDirEnvOverrideSubdir != "" {
				return filepath.Join(expandHome(raw, home), d.ConfigDirEnvOverrideSubdir)
			}
			return expandHome(raw, home)
		}
	}
	return filepath.Join(home, d.ConfigDir)
}

func (d agentHookDef) configPath() string {
	return filepath.Join(d.resolvedConfigDir(), d.ConfigFile)
}

func (d agentHookDef) ownedMarker() string {
	return "tmux-session-sidebar hooks " + d.Name
}

func (d agentHookDef) hookCommand(event agentHookEvent) string {
	return fmt.Sprintf(`: %s; sidebar_cli="${TMUX_SESSION_SIDEBAR_BIN:-$HOME/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar}"; if [ -z "$sidebar_cli" ] || [ ! -x "$sidebar_cli" ]; then sidebar_cli="$(command -v tmux-session-sidebar 2>/dev/null || true)"; fi; if [ -n "${TMUX_PANE:-}" ] && [ "${%s:-}" != "1" ] && [ -n "$sidebar_cli" ]; then "$sidebar_cli" hooks %s %s --pane "$TMUX_PANE" || true; fi`, d.ownedMarker(), d.DisableEnvVar, d.Name, event.SidebarEvent)
}

func expandHome(path string, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

func binaryOnPath(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}
