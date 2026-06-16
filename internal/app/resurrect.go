package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const resurrectInternalSidebarSession = "__tmux-session-sidebar"

type resurrectWindowKey struct {
	session string
	window  string
}

type resurrectPaneKey struct {
	resurrectWindowKey
	pane string
}

func resurrectPostSaveLayout(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("resurrect post-save-layout: missing save file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("resurrect post-save-layout: read %s: %w", path, err)
	}

	sidebarPanes, sidebarWindows := resurrectLiveSidebarPanes(ctx)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		switch fields[0] {
		case "pane":
			if resurrectLineSession(fields) == resurrectInternalSidebarSession {
				continue
			}
			if len(fields) > 5 {
				key := resurrectPaneKey{resurrectWindowKey: resurrectWindowKey{session: fields[1], window: fields[2]}, pane: fields[5]}
				if sidebarPanes[key] {
					continue
				}
			}
		case "window", "grouped_session":
			if resurrectReferencesInternalSession(fields) {
				continue
			}
			if fields[0] == "window" && len(fields) > 6 {
				key := resurrectWindowKey{session: fields[1], window: fields[2]}
				if layout := sidebarWindows[key]; layout != "" {
					fields[6] = layout
					line = strings.Join(fields, "\t")
				}
			}
		case "state":
			line = resurrectSanitizeStateLine(fields)
			if line == "" {
				continue
			}
		}
		kept = append(kept, line)
	}
	output := ""
	if len(kept) > 0 {
		output = strings.Join(kept, "\n") + "\n"
	}
	return writeFileAtomically(path, []byte(output), fileModeOrDefault(path, 0o600))
}

func resurrectLineSession(fields []string) string {
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

func resurrectReferencesInternalSession(fields []string) bool {
	if resurrectLineSession(fields) == resurrectInternalSidebarSession {
		return true
	}
	return len(fields) > 2 && fields[0] == "grouped_session" && fields[2] == resurrectInternalSidebarSession
}

func resurrectSanitizeStateLine(fields []string) string {
	if len(fields) < 3 {
		return strings.Join(fields, "\t")
	}
	if fields[1] == resurrectInternalSidebarSession && fields[2] == resurrectInternalSidebarSession {
		return ""
	}
	if fields[1] == resurrectInternalSidebarSession {
		fields[1] = fields[2]
	}
	if fields[2] == resurrectInternalSidebarSession {
		fields[2] = fields[1]
	}
	return strings.Join(fields, "\t")
}

func resurrectLiveSidebarPanes(ctx context.Context) (map[resurrectPaneKey]bool, map[resurrectWindowKey]string) {
	panes := map[resurrectPaneKey]bool{}
	windows := map[resurrectWindowKey]string{}
	out, err := tmux(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_index}\t#{pane_index}\t#{@session-sidebar-pane}\t#{window_id}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: resurrect list-panes failed, skipping live sidebar filtering: %v\n", err)
		return panes, windows
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 5 || !resurrectTmuxBool(fields[3]) {
			continue
		}
		window := resurrectWindowKey{session: fields[0], window: fields[1]}
		panes[resurrectPaneKey{resurrectWindowKey: window, pane: fields[2]}] = true
		if windows[window] != "" {
			continue
		}
		if layout := resurrectHiddenLayout(ctx, fields[4]); layout != "" {
			windows[window] = layout
		}
	}
	return panes, windows
}

// resurrectTmuxBool parses a tmux boolean format-string value (1/yes/true/on).
// Part of the transitional tmux command seam.
func resurrectTmuxBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "yes", "true", "on":
		return true
	default:
		return false
	}
}

func resurrectHiddenLayout(ctx context.Context, windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	out, err := tmux(ctx, "show-options", "-w", "-v", "-t", windowID, "@session-sidebar-window-layout")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func fileModeOrDefault(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
