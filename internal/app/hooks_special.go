package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func openCodePluginSource(def agentHookDef) string {
	return fmt.Sprintf(`// %s
// Derived from the cmux OpenCode hook integration design.
// Copyright (c) 2024-present Manaflow, Inc.
// cmux is dual-licensed GPL-3.0-or-later or commercial; see the cmux LICENSE for details.
// This tmux-session-sidebar generated file is an independent implementation inspired by that integration shape.
// DO NOT EDIT MANUALLY. Reinstall with: tmux-session-sidebar hooks %s install

import { spawnSync } from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

const TSS_PLUGIN_INSTALLED_KEY = Symbol.for("tmux-session-sidebar.session.plugin.installed");

function runtimePath() {
  const explicit = process.env.TMUX_SESSION_SIDEBAR_BIN;
  if (explicit && explicit.trim()) return explicit.trim();
  const bundled = path.join(os.homedir(), ".tmux", "plugins", "tmux-session-sidebar", ".bin", "tmux-session-sidebar");
  try {
    fs.accessSync(bundled, fs.constants.X_OK);
    return bundled;
  } catch (_) {}
  return "tmux-session-sidebar";
}

function currentPane() {
  const pane = process.env.TMUX_PANE;
  return pane && pane.trim() ? pane.trim() : null;
}

function sendHook(event) {
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

function eventProperties(event) {
  return (event && typeof event === "object" && event.properties) || {};
}

export const TMUXSessionSidebarPlugin = async () => {
  if (globalThis[TSS_PLUGIN_INSTALLED_KEY]) return {};
  globalThis[TSS_PLUGIN_INSTALLED_KEY] = true;
  return {
    event: async ({ event }) => {
      const props = eventProperties(event);
      switch (event && event.type) {
        case "session.created":
          sendHook("session-start");
          break;
        case "session.updated":
          if (props.info && props.info.time && props.info.time.archived) {
            sendHook("session-end");
          } else {
            sendHook("session-start");
          }
          break;
        case "session.status":
          if (props.status && props.status.type === "idle") {
            sendHook("stop");
          }
          break;
        case "session.idle":
          sendHook("stop");
          break;
        case "session.deleted":
          sendHook("session-end");
          break;
        default:
          break;
      }
    },
  };
};

export default TMUXSessionSidebarPlugin;
`, def.ownedMarker(), def.Name, def.DisableEnvVar, def.Name)
}

func piExtensionSource(def agentHookDef) string {
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

func ampPluginSource(def agentHookDef) string {
	return fmt.Sprintf(`// %s
// Derived from the cmux Amp plugin integration design.
// Copyright (c) 2024-present Manaflow, Inc.
// cmux is dual-licensed GPL-3.0-or-later or commercial; see the cmux LICENSE for details.
// This tmux-session-sidebar generated file is an independent implementation inspired by that integration shape.
// DO NOT EDIT MANUALLY. Reinstall with: tmux-session-sidebar hooks %s install

import type { PluginAPI } from "@ampcode/plugin";
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

export default function tmuxSessionSidebarAmpPlugin(amp: PluginAPI) {
  amp.on("session.start", async () => {
    sendHook("session-start");
  });
  amp.on("agent.start", async () => {
    sendHook("prompt-submit");
  });
  amp.on("agent.end", async () => {
    sendHook("stop");
  });
}
`, def.ownedMarker(), def.Name, def.DisableEnvVar, def.Name)
}

func installRovoHooks(stdout io.Writer, def agentHookDef, assumeYes bool) error {
	path := def.configPath()
	config, oldString, _, err := readYAMLConfig(path)
	if err != nil {
		return err
	}
	eventHooks := mapSlice(config, "eventHooks")
	events := sliceValue(eventHooks, "events")
	for _, hookEvent := range def.Events {
		events = upsertRovoEvent(def, events, hookEvent)
	}
	eventHooks["events"] = events
	config["eventHooks"] = eventHooks
	newString, err := prettyYAML(config)
	if err != nil {
		return err
	}
	if newString == oldString {
		fmt.Fprintf(stdout, "    %s hooks already up to date at %s\n", def.DisplayName, path)
		return nil
	}
	if !assumeYes {
		ok, err := confirmWrite(path)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stdout, "    Aborted.")
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(newString), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "    %s hooks installed at %s\n", def.DisplayName, path)
	return nil
}

func uninstallRovoHooks(stdout io.Writer, def agentHookDef) error {
	path := def.configPath()
	config, _, exists, err := readYAMLConfig(path)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintf(stdout, "    No %s hook config found at %s\n", def.DisplayName, path)
		return nil
	}
	eventHooks := mapSlice(config, "eventHooks")
	events := sliceValue(eventHooks, "events")
	changed := false
	rewritten := make([]any, 0, len(events))
	for _, rawEvent := range events {
		eventMap, ok := rawEvent.(map[string]any)
		if !ok {
			rewritten = append(rewritten, rawEvent)
			continue
		}
		commands := sliceValue(eventMap, "commands")
		rewrittenCommands := make([]any, 0, len(commands))
		for _, rawCommand := range commands {
			commandMap, ok := rawCommand.(map[string]any)
			if !ok {
				rewrittenCommands = append(rewrittenCommands, rawCommand)
				continue
			}
			if command, _ := commandMap["command"].(string); command != "" && containsOwnedMarker(command, def) {
				changed = true
				continue
			}
			rewrittenCommands = append(rewrittenCommands, rawCommand)
		}
		if len(rewrittenCommands) == 0 {
			changed = true
			continue
		}
		eventMap["commands"] = rewrittenCommands
		rewritten = append(rewritten, eventMap)
	}
	if !changed {
		fmt.Fprintf(stdout, "    Removed 0 tmux-session-sidebar hook(s) from %s\n", path)
		return nil
	}
	eventHooks["events"] = rewritten
	config["eventHooks"] = eventHooks
	newString, err := prettyYAML(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(newString), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "    Removed %s hooks from %s\n", def.DisplayName, path)
	return nil
}

func upsertRovoEvent(def agentHookDef, events []any, hookEvent agentHookEvent) []any {
	for index, rawEvent := range events {
		eventMap, ok := rawEvent.(map[string]any)
		if !ok {
			continue
		}
		name, _ := eventMap["name"].(string)
		if name != hookEvent.AgentEvent {
			continue
		}
		commands := sliceValue(eventMap, "commands")
		rewritten := make([]any, 0, len(commands)+1)
		for _, rawCommand := range commands {
			commandMap, ok := rawCommand.(map[string]any)
			if ok {
				if command, _ := commandMap["command"].(string); command != "" && containsOwnedMarker(command, def) {
					continue
				}
			}
			rewritten = append(rewritten, rawCommand)
		}
		rewritten = append(rewritten, map[string]any{"command": def.hookCommand(hookEvent)})
		eventMap["commands"] = rewritten
		events[index] = eventMap
		return events
	}
	return append(events, map[string]any{"name": hookEvent.AgentEvent, "commands": []any{map[string]any{"command": def.hookCommand(hookEvent)}}})
}

func readYAMLConfig(path string) (map[string]any, string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, "", false, nil
		}
		return nil, "", false, err
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return nil, "", true, fmt.Errorf("%s exists but is not valid YAML", path)
	}
	pretty, err := prettyYAML(decoded)
	if err != nil {
		return nil, "", true, err
	}
	return decoded, pretty, true, nil
}

func prettyYAML(v any) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func mapSlice(root map[string]any, key string) map[string]any {
	if value, ok := root[key].(map[string]any); ok {
		return value
	}
	result := map[string]any{}
	root[key] = result
	return result
}

func sliceValue(root map[string]any, key string) []any {
	if value, ok := root[key].([]any); ok {
		return value
	}
	return []any{}
}

func containsOwnedMarker(command string, def agentHookDef) bool {
	return strings.Contains(command, def.ownedMarker())
}
