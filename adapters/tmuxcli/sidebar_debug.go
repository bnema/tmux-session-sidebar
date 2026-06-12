package tmuxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	formatSidebarDebugWindow = "#{session_name}\t#{session_id}\t#{window_id}\t#{window_index}\t#{window_name}\t#{window_layout}\t#{window_width}\t#{window_height}"
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
	baselineSummary := "none"
	if raw, err := c.windowOptionValue(ctx, windowID, optionSidebarOpenWorkBaseline); err == nil {
		baselineSummary = summarizeSidebarOpenWorkBaseline(raw)
	}
	resizeSyncActive := false
	if raw, err := c.windowOptionValue(ctx, windowID, optionSidebarResizeSyncActive); err == nil {
		resizeSyncActive = parseTmuxBool(raw)
	}
	return formatSidebarDebugSnapshot(window, result.Stdout, baselineSummary, resizeSyncActive), nil
}

func formatSidebarDebugSnapshot(windowInfo string, panesInfo string, baselineSummary string, resizeSyncActive bool) string {
	windowFields := strings.Split(strings.TrimSpace(windowInfo), "\t")
	sessionName := debugField(windowFields, 0)
	sessionID := debugField(windowFields, 1)
	windowID := debugField(windowFields, 2)
	windowIndex := debugField(windowFields, 3)
	windowName := debugField(windowFields, 4)
	windowLayout := debugField(windowFields, 5)
	windowWidth := debugField(windowFields, 6)
	windowHeight := debugField(windowFields, 7)

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
	return fmt.Sprintf("session=%s(%s);window=%s:%s:%s;size=%sx%s;layout=%s;baseline=%s;sync=%t;panes=%s",
		sanitizeSidebarDebugValue(sessionName),
		sanitizeSidebarDebugValue(sessionID),
		sanitizeSidebarDebugValue(windowID),
		sanitizeSidebarDebugValue(windowIndex),
		sanitizeSidebarDebugValue(windowName),
		sanitizeSidebarDebugValue(windowWidth),
		sanitizeSidebarDebugValue(windowHeight),
		sanitizeSidebarDebugValue(windowLayout),
		sanitizeSidebarDebugValue(baselineSummary),
		resizeSyncActive,
		paneSummary,
	)
}

func summarizeSidebarOpenWorkBaseline(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "none"
	}
	var baseline sidebarOpenWorkBaseline
	if err := json.Unmarshal([]byte(raw), &baseline); err != nil {
		return "invalid"
	}
	if len(baseline.RepresentativePaneIDs) == 0 || len(baseline.RepresentativePaneIDs) != len(baseline.WorkWidths) {
		return "invalid"
	}
	parts := make([]string, 0, len(baseline.RepresentativePaneIDs))
	for i, repID := range baseline.RepresentativePaneIDs {
		repID = strings.TrimSpace(repID)
		if repID == "" || baseline.WorkWidths[i] <= 0 {
			return "invalid"
		}
		parts = append(parts, fmt.Sprintf("%s=%d", sanitizeSidebarDebugValue(repID), baseline.WorkWidths[i]))
	}
	return strings.Join(parts, ",")
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
