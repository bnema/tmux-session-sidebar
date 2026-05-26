package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

const (
	tmuxBinary = "tmux"

	cmdBreakPane      = "break-pane"
	cmdCapturePane    = "capture-pane"
	cmdDisplayMessage = "display-message"
	cmdJoinPane       = "join-pane"
	cmdKillPane       = "kill-pane"
	cmdListPanes      = "list-panes"
	cmdNewSession     = "new-session"
	cmdNewWindow      = "new-window"
	cmdResizePane     = "resize-pane"
	cmdRunShell       = "run-shell"
	cmdSelectLayout   = "select-layout"
	cmdSendKeys       = "send-keys"
	cmdSetOption      = "set-option"
	cmdShowOptions    = "show-options"

	formatPaneCurrentPath = "#{pane_current_path}"
	formatPaneID          = "#{pane_id}"
	formatSidebarPane     = "#{@session-sidebar-pane}"
	formatWindowID        = "#{window_id}"
	formatWindowLayout    = "#{window_layout}"

	escapedFormatPaneID = "##{pane_id}"

	optionSidebarPane         = "@session-sidebar-pane"
	optionSidebarWindowLayout = "@session-sidebar-window-layout"

	singletonSidebarSessionName = "__tmux-session-sidebar"
	singletonSidebarWindowName  = "sidebar"
)

type Client struct {
	Process ports.ProcessPort
}

func (c Client) LoadConfig(ctx context.Context) (ports.ConfigSnapshot, error) {
	key, err := c.option(ctx, "@session-sidebar-key")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	width, err := c.option(ctx, "@session-sidebar-width")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	roots, err := c.option(ctx, "@session-sidebar-project-roots")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	closeAfterSwitch, err := c.option(ctx, "@session-sidebar-close-after-switch")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	heatColors, err := c.option(ctx, "@session-sidebar-heat-colors")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	halfLifeHours, err := c.optionInt(ctx, "@session-sidebar-heat-half-life-hours")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	staleHours, err := c.optionInt(ctx, "@session-sidebar-heat-stale-hours")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	refreshSeconds, err := c.optionInt(ctx, "@session-sidebar-heat-refresh-seconds")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	recentHours, err := c.optionInt(ctx, "@session-sidebar-heat-recent-hours")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	activityDebugLog, err := c.option(ctx, "@session-sidebar-activity-debug-log")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	agentAttention, err := c.option(ctx, "@session-sidebar-agent-attention")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	return ports.ConfigSnapshot{
		Loaded:                true,
		KeyBinding:            key,
		Width:                 width,
		ProjectRoots:          splitProjectRoots(roots),
		CloseAfterSwitch:      parseTmuxBool(closeAfterSwitch),
		HeatColorsEnabled:     parseTmuxBool(heatColors),
		HeatHalfLifeHours:     halfLifeHours,
		HeatStaleHours:        staleHours,
		HeatRefreshSeconds:    refreshSeconds,
		HeatRecentHours:       recentHours,
		ActivityDebugLog:      parseTmuxBool(activityDebugLog),
		AgentAttentionEnabled: agentAttention == "" || parseTmuxBool(agentAttention),
	}, nil
}

func (c Client) ServerID(ctx context.Context) (string, error) {
	return c.display(ctx, "#{socket_path}")
}

func (c Client) ListSessions(ctx context.Context) ([]ports.TmuxSessionSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_windows}\t#{session_attached}"})
	if err != nil {
		return nil, err
	}
	var sessions []ports.TmuxSessionSnapshot
	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		windows, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		attached, err := strconv.Atoi(fields[3])
		if err != nil {
			continue
		}
		sessions = append(sessions, ports.TmuxSessionSnapshot{ID: fields[0], Name: fields[1], WindowCount: windows, AttachedCount: attached})
	}
	return sessions, nil
}

func (c Client) ListClients(ctx context.Context) ([]ports.TmuxClientSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"list-clients", "-F", "#{client_name}\t#{session_id}\t#{window_id}\t#{pane_id}\t#{client_session}"})
	if err != nil {
		return nil, err
	}
	var clients []ports.TmuxClientSnapshot
	for line := range strings.SplitSeq(strings.TrimRight(result.Stdout, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		clients = append(clients, ports.TmuxClientSnapshot{ID: fields[0], CurrentSessionID: fields[1], CurrentWindowID: fields[2], CurrentPaneID: fields[3], Attached: fields[4] != ""})
	}
	return clients, nil
}

func (c Client) ListPanes(ctx context.Context) ([]ports.TmuxPaneSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-F", "#{pane_id}\t#{session_id}\t#{session_name}\t#{window_id}\t#{pane_current_path}\t#{pane_current_command}\t#{pane_dead}\t#{pane_dead_status}\t#{@session-sidebar-pane}"})
	if err != nil {
		return nil, err
	}
	var panes []ports.TmuxPaneSnapshot
	for line := range strings.SplitSeq(strings.TrimRight(result.Stdout, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}
		panes = append(panes, ports.TmuxPaneSnapshot{
			PaneID:      fields[0],
			SessionID:   fields[1],
			SessionName: fields[2],
			WindowID:    fields[3],
			CurrentPath: fields[4],
			CurrentCmd:  fields[5],
			Dead:        parseTmuxBool(fields[6]),
			DeadStatus:  fields[7],
			Sidebar:     parseTmuxBool(fields[8]),
		})
	}
	return panes, nil
}

func (c Client) CapturePaneText(ctx context.Context, paneID string, tailLines int) (string, error) {
	if tailLines <= 0 {
		tailLines = 1
	}
	start := -tailLines
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdCapturePane, "-pJ", "-t", strings.TrimSpace(paneID), "-S", strconv.Itoa(start), "-E", "-1"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (c Client) CurrentPanePath(ctx context.Context, clientID string) (string, error) {
	return c.displayTarget(ctx, clientID, formatPaneCurrentPath)
}

func (c Client) SessionPath(ctx context.Context, sessionName string) (string, error) {
	return c.displayTarget(ctx, "="+sessionName, formatPaneCurrentPath)
}

func (c Client) WindowID(ctx context.Context, target string) (string, error) {
	windowID, err := c.displayTarget(ctx, target, formatWindowID)
	if err != nil {
		return "", err
	}
	if windowID == "" {
		return "", fmt.Errorf("resolve tmux window id for target %q: empty output", strings.TrimSpace(target))
	}
	return windowID, nil
}

func (c Client) FindSidebarPane(ctx context.Context, target string) (ports.PaneRef, error) {
	windowID, err := c.WindowID(ctx, target)
	if err != nil {
		return ports.PaneRef{}, err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-t", windowID, "-F", formatPaneID + "\t" + formatSidebarPane})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == "1" {
			return ports.PaneRef{PaneID: fields[0], WindowID: windowID}, nil
		}
	}
	return ports.PaneRef{WindowID: windowID}, nil
}

func (c Client) CloseAfterSwitch(ctx context.Context) (bool, error) {
	config, err := c.LoadConfig(ctx)
	return config.CloseAfterSwitch, err
}

func (c Client) PaneSize(ctx context.Context, paneID string) (ports.PaneSize, error) {
	out, err := c.displayTarget(ctx, paneID, "#{pane_width}\t#{pane_height}")
	if err != nil {
		return ports.PaneSize{}, err
	}
	fields := strings.Split(out, "\t")
	width, err := strconv.Atoi(fields[0])
	if err != nil {
		return ports.PaneSize{}, err
	}
	height := 0
	if len(fields) > 1 {
		height, err = strconv.Atoi(fields[1])
		if err != nil {
			return ports.PaneSize{}, err
		}
	}
	return ports.PaneSize{Width: width, Height: height}, nil
}

func (c Client) SwitchClientSession(ctx context.Context, clientID string, sessionName string) error {
	args := []string{"switch-client"}
	if clientID != "" {
		args = append(args, "-c", clientID)
	}
	args = append(args, "-t", sessionName)
	_, err := c.Process.Exec(ctx, tmuxBinary, args)
	return err
}

func (c Client) DisplayMessage(ctx context.Context, clientID string, message string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdDisplayMessage, "-c", clientID, message})
	return err
}

func (c Client) CreateSession(ctx context.Context, sessionName string, path string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{"new-session", "-d", "-s", sessionName, "-c", path})
	return err
}

func (c Client) RenameSession(ctx context.Context, oldName string, newName string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{"rename-session", "-t", "=" + oldName, newName})
	return err
}

func (c Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{"kill-session", "-t", "=" + sessionName})
	return err
}

func (c Client) markSidebarPane(ctx context.Context, paneID string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSetOption, "-p", "-t", strings.TrimSpace(paneID), optionSidebarPane, "1"})
	return err
}

func (c Client) resizePaneWidth(ctx context.Context, paneID string, width string) error {
	width = strings.TrimSpace(width)
	if width == "" {
		return nil
	}
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdResizePane, "-t", strings.TrimSpace(paneID), "-x", width})
	return err
}

func (c Client) RefreshSidebar(ctx context.Context, clientID string) error {
	if strings.TrimSpace(clientID) == "" {
		return nil
	}
	pane, err := c.FindSidebarPane(ctx, clientID)
	if err != nil || pane.PaneID == "" {
		return err
	}
	_, err = c.Process.Exec(ctx, tmuxBinary, []string{cmdSendKeys, "-t", pane.PaneID, "F5"})
	return err
}

func (c Client) SaveWindowLayout(ctx context.Context, windowID string) error {
	if strings.TrimSpace(windowID) == "" {
		return nil
	}
	layout, err := c.displayTarget(ctx, windowID, formatWindowLayout)
	if err != nil {
		return err
	}
	layout = strings.TrimSpace(layout)
	if layout == "" {
		return nil
	}
	_, err = c.Process.Exec(ctx, tmuxBinary, []string{cmdSetOption, "-wq", "-t", windowID, optionSidebarWindowLayout, layout})
	return err
}

func (c Client) RestoreWindowLayout(ctx context.Context, windowID string) error {
	if strings.TrimSpace(windowID) == "" {
		return nil
	}
	layout, err := c.savedWindowLayout(ctx, windowID)
	if err != nil {
		return err
	}
	if layout == "" {
		return nil
	}
	_, selectErr := c.Process.Exec(ctx, tmuxBinary, []string{cmdSelectLayout, "-t", windowID, layout})
	if selectErr != nil {
		return selectErr
	}
	_, unsetErr := c.Process.Exec(ctx, tmuxBinary, []string{cmdSetOption, "-wu", "-t", windowID, optionSidebarWindowLayout})
	return unsetErr
}

func (c Client) savedWindowLayout(ctx context.Context, windowID string) (string, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdShowOptions, "-w", "-v", "-t", windowID, optionSidebarWindowLayout})
	if err != nil {
		if isTmuxMissingOption(result) {
			return "", nil
		}
		return "", err
	}
	layout := strings.TrimSpace(result.Stdout)
	if layout == "" {
		return "", nil
	}
	return layout, nil
}

func isTmuxMissingOption(result ports.Result) bool {
	message := strings.ToLower(result.Stderr + result.Stdout)
	return strings.Contains(message, "invalid option") || strings.Contains(message, "unknown option")
}

func (c Client) ScheduleSidebarRestoreOnExit(ctx context.Context, clientID string, paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		// The sidebar pane can already be gone during TUI shutdown; nothing to schedule then.
		pane, err := c.FindSidebarPane(ctx, clientID)
		if err != nil {
			if isTmuxTargetGone(err) {
				return nil
			}
			return err
		}
		paneID = strings.TrimSpace(pane.PaneID)
	}
	if paneID == "" {
		return nil
	}
	windowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(windowID) == "" {
		// Exit cleanup is best-effort because the pane/window may disappear first.
		return nil
	}
	layout, err := c.savedWindowLayout(ctx, windowID)
	if err != nil {
		return err
	}
	if layout == "" {
		// No saved layout means there is no sidebar-induced split to restore.
		return nil
	}
	_, err = c.Process.Exec(ctx, tmuxBinary, []string{cmdRunShell, "-b", sidebarLayoutRestoreCommand(windowID, paneID)})
	return err
}

func (c Client) LoadSessionMetadata(ctx context.Context, sessionName string) (ports.SessionMetadata, error) {
	kind, err := c.displayTarget(ctx, sessionName, "#{@session-sidebar-kind}")
	if err != nil {
		return ports.SessionMetadata{}, err
	}
	projectPath, err := c.displayTarget(ctx, sessionName, "#{@session-sidebar-project-path}")
	if err != nil {
		return ports.SessionMetadata{}, err
	}
	return ports.SessionMetadata{Kind: kind, ProjectPath: projectPath}, nil
}

func (c Client) SaveSessionMetadata(ctx context.Context, sessionName string, metadata ports.SessionMetadata) error {
	if _, err := c.Process.Exec(ctx, "tmux", []string{"set-option", "-t", sessionName, "@session-sidebar-kind", metadata.Kind}); err != nil {
		return err
	}
	_, err := c.Process.Exec(ctx, "tmux", []string{"set-option", "-t", sessionName, "@session-sidebar-project-path", metadata.ProjectPath})
	return err
}

func parseTmuxBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "yes", "true", "on":
		return true
	default:
		return false
	}
}

func splitProjectRoots(roots string) []string {
	parts := strings.Split(roots, ":")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func (c Client) option(ctx context.Context, name string) (string, error) {
	result, err := c.Process.Exec(ctx, "tmux", []string{"show-options", "-gvq", name})
	return strings.TrimSpace(result.Stdout), err
}

func (c Client) optionInt(ctx context.Context, name string) (int, error) {
	value, err := c.option(ctx, name)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (c Client) display(ctx context.Context, format string) (string, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdDisplayMessage, "-p", format})
	if err != nil {
		return "", wrapTmuxError(result, err)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (c Client) displayTarget(ctx context.Context, target string, format string) (string, error) {
	args := []string{cmdDisplayMessage, "-p"}
	if strings.TrimSpace(target) != "" {
		args = append(args, tmuxTargetFlag(target), target)
	}
	args = append(args, format)
	result, err := c.Process.Exec(ctx, tmuxBinary, args)
	if err != nil {
		return "", wrapTmuxError(result, err)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func tmuxTargetFlag(target string) string {
	if strings.HasPrefix(strings.TrimSpace(target), "/dev/") {
		return "-c"
	}
	return "-t"
}

type tmuxError struct {
	result ports.Result
	err    error
}

func (e tmuxError) Error() string {
	return e.err.Error()
}

func (e tmuxError) Unwrap() error {
	return e.err
}

func wrapTmuxError(result ports.Result, err error) error {
	if err == nil {
		return nil
	}
	return tmuxError{result: result, err: err}
}

func isTmuxTargetGone(err error) bool {
	var tmuxErr tmuxError
	if !errors.As(err, &tmuxErr) {
		return false
	}
	message := strings.ToLower(tmuxErr.result.Stderr + tmuxErr.result.Stdout)
	return strings.Contains(message, "no such window") ||
		strings.Contains(message, "can't find window") ||
		strings.Contains(message, "no such pane") ||
		strings.Contains(message, "can't find pane") ||
		strings.Contains(message, "no such client") ||
		strings.Contains(message, "can't find client")
}

func parsePaneRef(out string) (ports.PaneRef, error) {
	if out == "" {
		return ports.PaneRef{}, fmt.Errorf("open sidebar pane: empty tmux output")
	}
	fields := strings.Split(out, "\t")
	if len(fields) < 2 || fields[0] == "" || fields[1] == "" {
		return ports.PaneRef{}, fmt.Errorf("open sidebar pane: malformed tmux output %q", out)
	}
	return ports.PaneRef{PaneID: fields[0], WindowID: fields[1]}, nil
}

func sidebarLayoutRestoreCommand(windowID string, paneID string) string {
	script := "window=$1; pane=$2; option=$3; " +
		"for _ in 1 2 3 4 5 6 7 8 9 10 " +
		"11 12 13 14 15 16 17 18 19 20 " +
		"21 22 23 24 25 26 27 28 29 30 " +
		"31 32 33 34 35 36 37 38 39 40 " +
		"41 42 43 44 45 46 47 48 49 50; do " +
		"tmux list-panes -t \"$window\" -F '" + escapedFormatPaneID + "' 2>/dev/null | grep -Fxq \"$pane\" || break; " +
		"sleep 0.05; " +
		"done; " +
		"tmux list-panes -t \"$window\" -F '" + escapedFormatPaneID + "' 2>/dev/null | grep -Fxq \"$pane\" && exit 0; " +
		"layout=$(tmux show-options -w -v -t \"$window\" \"$option\" 2>/dev/null || true); " +
		"[ -n \"$layout\" ] || exit 0; " +
		"tmux select-layout -t \"$window\" \"$layout\" >/dev/null 2>&1 && " +
		"tmux set-option -wu -t \"$window\" \"$option\" >/dev/null 2>&1 || true"
	return "sh -c " + shellQuote(script) + " sh " + shellQuote(windowID) + " " + shellQuote(paneID) + " " + shellQuote(optionSidebarWindowLayout)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
