package tmuxcli

import (
	"context"
	"fmt"
	"strings"
)

const (
	formatSidebarDebugWindow = "#{session_name}\t#{session_id}\t#{window_id}\t#{window_index}\t#{window_name}\t#{window_layout}"
	formatSidebarDebugPane   = "#{pane_id}\t#{pane_index}\t#{pane_width}\t#{pane_height}\t#{pane_left}\t#{pane_top}\t#{pane_active}\t#{pane_dead}\t#{pane_current_command}\t#{@session-sidebar-pane}"
)

func (c Client) SidebarDebugSnapshot(ctx context.Context, windowID string) (string, error) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return "", nil
	}
	window, err := c.displayTarget(ctx, windowID, formatSidebarDebugWindow)
	if err != nil {
		return "", err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-t", windowID, "-F", formatSidebarDebugPane})
	if err != nil {
		return "", wrapTmuxError(result, err)
	}
	return formatSidebarDebugSnapshot(window, result.Stdout), nil
}

func formatSidebarDebugSnapshot(windowInfo string, panesInfo string) string {
	windowFields := strings.Split(strings.TrimSpace(windowInfo), "\t")
	sessionName := debugField(windowFields, 0)
	sessionID := debugField(windowFields, 1)
	windowID := debugField(windowFields, 2)
	windowIndex := debugField(windowFields, 3)
	windowName := debugField(windowFields, 4)
	windowLayout := debugField(windowFields, 5)

	panes := make([]string, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(panesInfo), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		panes = append(panes, fmt.Sprintf("%s[idx=%s,size=%sx%s,pos=%s,%s,active=%t,dead=%t,sidebar=%t,cmd=%s]",
			sanitizeSidebarDebugValue(debugField(fields, 0)),
			sanitizeSidebarDebugValue(debugField(fields, 1)),
			sanitizeSidebarDebugValue(debugField(fields, 2)),
			sanitizeSidebarDebugValue(debugField(fields, 3)),
			sanitizeSidebarDebugValue(debugField(fields, 4)),
			sanitizeSidebarDebugValue(debugField(fields, 5)),
			parseTmuxBool(debugField(fields, 6)),
			parseTmuxBool(debugField(fields, 7)),
			parseTmuxBool(debugField(fields, 9)),
			sanitizeSidebarDebugValue(debugField(fields, 8)),
		))
	}
	paneSummary := "none"
	if len(panes) > 0 {
		paneSummary = strings.Join(panes, ",")
	}
	return fmt.Sprintf("session=%s(%s);window=%s:%s:%s;layout=%s;panes=%s",
		sanitizeSidebarDebugValue(sessionName),
		sanitizeSidebarDebugValue(sessionID),
		sanitizeSidebarDebugValue(windowID),
		sanitizeSidebarDebugValue(windowIndex),
		sanitizeSidebarDebugValue(windowName),
		sanitizeSidebarDebugValue(windowLayout),
		paneSummary,
	)
}

func debugField(fields []string, index int) string {
	if index < 0 || index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func sanitizeSidebarDebugValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	replacer := strings.NewReplacer(
		`\`, `\\`,
		" ", "_",
		"\n", "|",
		"\t", "|",
		";", `\;`,
		":", `\:`,
		"=", `\=`,
		"(", `\(`,
		")", `\)`,
		",", `\,`,
	)
	return replacer.Replace(value)
}
