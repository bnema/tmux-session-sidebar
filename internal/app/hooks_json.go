package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func installJSONHooks(stdout io.Writer, def agentHookDef, assumeYes bool) error {
	path := def.configPath()
	current, oldString, err := readJSONConfig(path)
	if err != nil {
		return err
	}
	hooks, _ := current["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, value := range hooks {
		switch def.Format {
		case agentHookFormatFlatJSON:
			entries, ok := value.([]any)
			if !ok {
				continue
			}
			rewritten := make([]any, 0, len(entries))
			for _, entry := range entries {
				object, ok := entry.(map[string]any)
				if !ok || !jsonHookObjectOwned(object, def) {
					rewritten = append(rewritten, entry)
				}
			}
			if len(rewritten) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = rewritten
			}
		case agentHookFormatNestedJSON:
			groups, ok := value.([]any)
			if !ok {
				continue
			}
			rewrittenGroups := make([]any, 0, len(groups))
			for _, group := range groups {
				object, ok := group.(map[string]any)
				if !ok {
					rewrittenGroups = append(rewrittenGroups, group)
					continue
				}
				hookList, ok := object["hooks"].([]any)
				if !ok {
					rewrittenGroups = append(rewrittenGroups, group)
					continue
				}
				rewrittenHooks := make([]any, 0, len(hookList))
				for _, hook := range hookList {
					hookObject, ok := hook.(map[string]any)
					if !ok || !jsonHookObjectOwned(hookObject, def) {
						rewrittenHooks = append(rewrittenHooks, hook)
					}
				}
				if len(rewrittenHooks) == 0 {
					continue
				}
				object["hooks"] = rewrittenHooks
				rewrittenGroups = append(rewrittenGroups, object)
			}
			if len(rewrittenGroups) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = rewrittenGroups
			}
		}
	}
	for _, event := range def.Events {
		command := def.hookCommand(event)
		switch def.Format {
		case agentHookFormatFlatJSON:
			entries, _ := hooks[event.AgentEvent].([]any)
			entries = append(entries, map[string]any{"command": command})
			hooks[event.AgentEvent] = entries
		case agentHookFormatNestedJSON:
			groups, _ := hooks[event.AgentEvent].([]any)
			groups = append(groups, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 5000}}})
			hooks[event.AgentEvent] = groups
		}
	}
	current["hooks"] = hooks
	if def.Format == agentHookFormatFlatJSON {
		current["version"] = 1
	}
	newString, err := prettyJSON(current)
	if err != nil {
		return err
	}
	if newString == oldString {
		return writef(stdout, "    %s hooks already up to date at %s\n", def.DisplayName, path)
	}
	if !assumeYes {
		ok, err := confirmWrite(path)
		if err != nil {
			return err
		}
		if !ok {
			return writeln(stdout, "    Aborted.")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(newString), 0o644); err != nil {
		return err
	}
	return writef(stdout, "    %s hooks installed at %s\n", def.DisplayName, path)
}

func uninstallJSONHooks(stdout io.Writer, def agentHookDef) error {
	path := def.configPath()
	current, _, err := readJSONConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writef(stdout, "    No %s hook config found at %s\n", def.DisplayName, path)
		}
		return err
	}
	hooks, _ := current["hooks"].(map[string]any)
	if hooks == nil {
		return writef(stdout, "    Removed 0 tmux-session-sidebar hook(s) from %s\n", path)
	}
	changed := false
	for event, value := range hooks {
		switch def.Format {
		case agentHookFormatFlatJSON:
			entries, ok := value.([]any)
			if !ok {
				continue
			}
			rewritten := make([]any, 0, len(entries))
			for _, entry := range entries {
				object, ok := entry.(map[string]any)
				if ok && jsonHookObjectOwned(object, def) {
					changed = true
					continue
				}
				rewritten = append(rewritten, entry)
			}
			if len(rewritten) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = rewritten
			}
		case agentHookFormatNestedJSON:
			groups, ok := value.([]any)
			if !ok {
				continue
			}
			rewrittenGroups := make([]any, 0, len(groups))
			for _, group := range groups {
				object, ok := group.(map[string]any)
				if !ok {
					rewrittenGroups = append(rewrittenGroups, group)
					continue
				}
				hookList, ok := object["hooks"].([]any)
				if !ok {
					rewrittenGroups = append(rewrittenGroups, group)
					continue
				}
				rewrittenHooks := make([]any, 0, len(hookList))
				for _, hook := range hookList {
					hookObject, ok := hook.(map[string]any)
					if ok && jsonHookObjectOwned(hookObject, def) {
						changed = true
						continue
					}
					rewrittenHooks = append(rewrittenHooks, hook)
				}
				if len(rewrittenHooks) == 0 {
					continue
				}
				object["hooks"] = rewrittenHooks
				rewrittenGroups = append(rewrittenGroups, object)
			}
			if len(rewrittenGroups) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = rewrittenGroups
			}
		}
	}
	if !changed {
		return writef(stdout, "    Removed 0 tmux-session-sidebar hook(s) from %s\n", path)
	}
	current["hooks"] = hooks
	newString, err := prettyJSON(current)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(newString), 0o644); err != nil {
		return err
	}
	return writef(stdout, "    Removed %s hooks from %s\n", def.DisplayName, path)
}

func readJSONConfig(path string) (map[string]any, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, "", nil
		}
		return nil, "", err
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, "", fmt.Errorf("%s exists but is not valid JSON", path)
	}
	pretty, err := prettyJSON(decoded)
	if err != nil {
		return nil, "", err
	}
	return decoded, pretty, nil
}

func prettyJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(data, '\n')), nil
}

func jsonHookObjectOwned(object map[string]any, def agentHookDef) bool {
	command, _ := object["command"].(string)
	return strings.Contains(command, def.ownedMarker())
}
