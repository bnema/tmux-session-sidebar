package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
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
	cmdSelectPane     = "select-pane"
	cmdSendKeys       = "send-keys"
	cmdSetOption      = "set-option"
	cmdShowOptions    = "show-options"
	cmdSwitchClient   = "switch-client"

	formatPaneCurrentPath = "#{pane_current_path}"
	formatPaneID          = "#{pane_id}"
	formatSidebarPane     = "#{@session-sidebar-pane}"
	formatWindowID        = "#{window_id}"
	formatWindowLayout    = "#{window_layout}"

	configQuerySeparator = "__TMUX_SESSION_SIDEBAR_CFG_SEP__"

	escapedFormatPaneID = "##{pane_id}"

	optionSidebarPane             = "@session-sidebar-pane"
	optionSidebarWindowLayout     = "@session-sidebar-window-layout"
	optionSidebarOpenWorkBaseline = "@session-sidebar-open-work-baseline"
	optionSidebarResizeSyncActive = "@session-sidebar-resize-sync-active"

	singletonSidebarSessionName = "__tmux-session-sidebar"
	singletonSidebarWindowName  = "sidebar"
)

// Compile-time interface assertions verifying tmuxcli.Client satisfies the
// multiplexer-generic port interfaces. These catch interface drift early.
var _ ports.RuntimePort = Client{}
var _ ports.SidebarSwitchPort = Client{}
var _ ports.SidebarFollowPort = Client{}
var _ ports.SidebarResizePort = Client{}

type Client struct {
	Process ports.ProcessPort
	Logger  ports.LoggerPort
}

func (c Client) Run(ctx context.Context, args []string) (ports.Result, error) {
	return c.Process.Exec(ctx, tmuxBinary, args)
}

func (c Client) LoadConfig(ctx context.Context) (ports.ConfigSnapshot, error) {
	opts, err := c.loadOptionsMap(ctx)
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}

	get := func(name string) string { return opts[name] }
	getInt := func(name string) (int, error) {
		value := opts[name]
		if value == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, err
		}
		return parsed, nil
	}

	halfLifeHours, err := getInt("@session-sidebar-heat-half-life-hours")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	staleHours, err := getInt("@session-sidebar-heat-stale-hours")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	refreshSeconds, err := getInt("@session-sidebar-heat-refresh-seconds")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	maxHighlighted, err := getInt("@session-sidebar-heat-max-highlighted")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}
	continuumGraceSeconds, err := getInt("@session-sidebar-continuum-grace-seconds")
	if err != nil {
		return ports.ConfigSnapshot{}, err
	}

	key := get("@session-sidebar-key")
	width := get("@session-sidebar-width")
	roots := get("@session-sidebar-project-roots")
	closeAfterSwitch := get("@session-sidebar-close-after-switch")
	heatColors := get("@session-sidebar-heat-colors")
	recent := get("@session-sidebar-heat-recent")
	recentInterval := parseHeatRecentInterval(recent)
	activityDebugLog := get("@session-sidebar-activity-debug-log")
	agentAttention := get("@session-sidebar-agent-attention")
	agentAttentionAnimation := get("@session-sidebar-agent-attention-animation")
	autoSortRecent := get("@session-sidebar-auto-sort-recent")
	autoSortRecentInterval := parseAutoSortRecentInterval(autoSortRecent)
	restoreSessionsMode := get("@session-sidebar-restore-sessions")
	colorSchemeMode := get("@session-sidebar-color-scheme")
	metadataSubline := get("@session-sidebar-metadata-subline")
	metadataInactive := get("@session-sidebar-metadata-inactive")

	return ports.ConfigSnapshot{
		Loaded:                  true,
		KeyBinding:              key,
		Width:                   width,
		ProjectRoots:            splitProjectRoots(roots),
		CloseAfterSwitch:        parseTmuxBool(closeAfterSwitch),
		HeatColorsEnabled:       parseTmuxBool(heatColors),
		HeatHalfLifeHours:       halfLifeHours,
		HeatStaleHours:          staleHours,
		HeatRefreshSeconds:      refreshSeconds,
		HeatRecentInterval:      recentInterval,
		HeatMaxHighlighted:      maxHighlighted,
		ActivityDebugLog:        parseTmuxBool(activityDebugLog),
		AgentAttentionEnabled:   agentAttention == "" || parseTmuxBool(agentAttention),
		AgentAttentionAnimation: config.ParseAgentAttentionAnimation(agentAttentionAnimation),
		AutoSortRecentInterval:  autoSortRecentInterval,
		RestoreSessionsMode:     normalizeRestoreSessionsMode(restoreSessionsMode),
		ContinuumGraceSeconds:   continuumGraceSeconds,
		ColorSchemeMode:         config.ParseColorSchemeMode(colorSchemeMode),
		MetadataSublineEnabled:  metadataSubline == "" || parseTmuxBool(metadataSubline),
		MetadataInactiveEnabled: metadataInactive == "" || parseTmuxBool(metadataInactive),
	}, nil
}

func (c Client) ServerID(ctx context.Context) (string, error) {
	return c.display(ctx, "#{socket_path}")
}

func (c Client) ListSessions(ctx context.Context) ([]ports.SessionSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_windows}\t#{session_attached}"})
	if err != nil {
		return nil, err
	}
	var sessions []ports.SessionSnapshot
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
		sessions = append(sessions, ports.SessionSnapshot{ID: fields[0], Name: fields[1], WindowCount: windows, AttachedCount: attached})
	}
	return sessions, nil
}

func (c Client) ListClients(ctx context.Context) ([]ports.ClientSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"list-clients", "-F", "#{client_name}\t#{session_id}\t#{window_id}\t#{pane_id}\t#{client_session}"})
	if err != nil {
		return nil, err
	}
	var clients []ports.ClientSnapshot
	for line := range strings.SplitSeq(strings.TrimRight(result.Stdout, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		clients = append(clients, ports.ClientSnapshot{ID: fields[0], CurrentSessionID: fields[1], CurrentWindowID: fields[2], CurrentPaneID: fields[3], Attached: fields[4] != ""})
	}
	return clients, nil
}

func (c Client) ListPanes(ctx context.Context) ([]ports.PaneSnapshot, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-F", "#{pane_id}\t#{session_id}\t#{session_name}\t#{window_id}\t#{pane_current_path}\t#{pane_current_command}\t#{pane_dead}\t#{pane_dead_status}\t#{@session-sidebar-pane}"})
	if err != nil {
		return nil, err
	}
	var panes []ports.PaneSnapshot
	for line := range strings.SplitSeq(strings.TrimRight(result.Stdout, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}
		panes = append(panes, ports.PaneSnapshot{
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
	return c.displayTarget(ctx, exactSessionWindowTarget(sessionName), formatPaneCurrentPath)
}

func (c Client) SessionPaths(ctx context.Context, sessionNames []string) (map[string]string, error) {
	requested := make(map[string]struct{}, len(sessionNames))
	for _, name := range sessionNames {
		if name != "" {
			requested[name] = struct{}{}
		}
	}
	if len(requested) == 0 {
		return map[string]string{}, nil
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-F", "#{session_name}\t#{window_active}\t#{pane_active}\t#{pane_current_path}"})
	if err != nil {
		return nil, wrapTmuxError(result, err)
	}
	paths := make(map[string]string, len(requested))
	for line := range strings.SplitSeq(strings.TrimRight(result.Stdout, "\n"), "\n") {
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 4 || fields[1] != "1" || fields[2] != "1" {
			continue
		}
		name := fields[0]
		if _, ok := requested[name]; !ok {
			continue
		}
		if path := strings.TrimSpace(fields[3]); path != "" {
			paths[name] = path
		}
	}
	return paths, nil
}

func (c Client) WindowID(ctx context.Context, target string) (string, error) {
	windowID, err := c.displayTarget(ctx, target, formatWindowID)
	if err != nil {
		return "", err
	}
	trimmedTarget := strings.TrimSpace(target)
	if windowID == "" {
		if concreteTmuxTarget(trimmedTarget) {
			return "", fmt.Errorf("resolve tmux window id for target %q: %w", trimmedTarget, ports.ErrMultiplexerTargetGone)
		}
		return "", fmt.Errorf("resolve tmux window id for target %q: empty output", trimmedTarget)
	}
	return windowID, nil
}

func (c Client) FindSidebarPane(ctx context.Context, target string) (ports.PaneRef, error) {
	windowID, err := c.WindowID(ctx, target)
	if err != nil {
		return ports.PaneRef{}, err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-t", windowID, "-F", formatPaneID + "\t" + formatSidebarPane + "\t#{pane_dead}"})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 && fields[1] == "1" && !parseTmuxBool(fields[2]) {
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
	args := switchClientArgs(clientID, exactSessionWindowTarget(sessionName))
	_, err := c.Process.Exec(ctx, tmuxBinary, args)
	return err
}

func (c Client) switchClientToExactTarget(ctx context.Context, clientID string, target string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, switchClientArgs(clientID, target))
	return err
}

func switchClientArgs(clientID string, target string) []string {
	args := []string{cmdSwitchClient}
	if clientID != "" {
		args = append(args, "-c", clientID)
	}
	return append(args, "-t", target)
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

func (c Client) selectPane(ctx context.Context, paneID string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSelectPane, "-t", strings.TrimSpace(paneID)})
	return err
}

func (c Client) selectPaneRightOf(ctx context.Context, paneID string) error {
	// tmux -R is intentionally best-effort: if there is no pane to the right,
	// tmux keeps the current selection, which is still preferable to focusing
	// the sidebar pane after it is attached.
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSelectPane, "-t", strings.TrimSpace(paneID), "-R"})
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
	return c.refreshSidebarPane(ctx, pane.PaneID)
}

func (c Client) RefreshAllSidebars(ctx context.Context) error {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-f", "#{==:#{@session-sidebar-pane},1}", "-F", formatPaneID})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	var firstErr error
	for paneID := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		paneID = strings.TrimSpace(paneID)
		if paneID == "" {
			continue
		}
		if err := c.refreshSidebarPane(ctx, paneID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c Client) refreshSidebarPane(ctx context.Context, paneID string) error {
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSendKeys, "-t", paneID, "F5"})
	return err
}

func (c Client) ClearSavedWindowLayout(ctx context.Context, windowID string) error {
	return c.clearWindowLayout(ctx, windowID, optionSidebarWindowLayout)
}

func (c Client) RestoreWindowLayout(ctx context.Context, windowID string) error {
	return c.restoreWindowLayout(ctx, windowID, optionSidebarWindowLayout)
}

func (c Client) captureWindowLayout(ctx context.Context, windowID string, option string) error {
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
	return c.setWindowOptionValue(ctx, windowID, option, layout)
}

func (c Client) clearWindowLayout(ctx context.Context, windowID string, option string) error {
	return c.clearWindowOptionValue(ctx, windowID, option)
}

func (c Client) restoreWindowLayout(ctx context.Context, windowID string, option string) error {
	if strings.TrimSpace(windowID) == "" {
		return nil
	}
	layout, err := c.windowLayoutOption(ctx, windowID, option)
	if err != nil {
		return err
	}
	if layout == "" {
		return nil
	}
	result, selectErr := c.Process.Exec(ctx, tmuxBinary, []string{cmdSelectLayout, "-t", windowID, layout})
	if selectErr != nil {
		return wrapTmuxError(result, selectErr)
	}
	return c.clearWindowLayout(ctx, windowID, option)
}

func (c Client) savedWindowLayout(ctx context.Context, windowID string) (string, error) {
	return c.windowLayoutOption(ctx, windowID, optionSidebarWindowLayout)
}

func (c Client) windowLayoutOption(ctx context.Context, windowID string, option string) (string, error) {
	return c.windowOptionValue(ctx, windowID, option)
}

func (c Client) setWindowOptionValue(ctx context.Context, windowID string, option string, value string) error {
	if strings.TrimSpace(windowID) == "" {
		return nil
	}
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSetOption, "-wq", "-t", windowID, option, value})
	return err
}

func (c Client) clearWindowOptionValue(ctx context.Context, windowID string, option string) error {
	if strings.TrimSpace(windowID) == "" {
		return nil
	}
	_, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdSetOption, "-wu", "-t", windowID, option})
	return err
}

func (c Client) windowOptionValue(ctx context.Context, windowID string, option string) (string, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdShowOptions, "-w", "-v", "-t", windowID, option})
	if err != nil {
		if isTmuxMissingOption(result) {
			return "", nil
		}
		return "", wrapTmuxError(result, err)
	}
	value := strings.TrimSpace(result.Stdout)
	if value == "" {
		return "", nil
	}
	return value, nil
}

func isTmuxMissingOption(result ports.Result) bool {
	message := strings.ToLower(result.Stderr + result.Stdout)
	return strings.Contains(message, "invalid option") || strings.Contains(message, "unknown option")
}

func (c Client) ScheduleSidebarRestoreOnExit(ctx context.Context, clientID string, paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		// The sidebar pane can already be gone during TUI shutdown. When that
		// happens, clear any stale saved hidden-layout option for the affected
		// window and stop because there is no pane left to watch asynchronously.
		pane, err := c.FindSidebarPane(ctx, clientID)
		if err != nil {
			if isTmuxTargetGone(err) {
				return nil
			}
			return err
		}
		paneID = strings.TrimSpace(pane.PaneID)
		if paneID == "" {
			if err := c.ClearSavedWindowLayout(ctx, pane.WindowID); err != nil {
				if isTmuxTargetGone(err) {
					return nil
				}
				return err
			}
			return nil
		}
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
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if layout == "" {
		// No saved layout means there is no sidebar-induced split to restore.
		return nil
	}
	// No layout replay is needed here. Once the sidebar pane exits, tmux
	// redistributes the remaining panes natively. The background command only
	// waits for the sidebar pane to disappear, then clears the stale saved
	// hidden-layout option best-effort.
	_, err = c.Process.Exec(ctx, tmuxBinary, []string{cmdRunShell, "-b", sidebarLayoutCleanupCommand(windowID, paneID)})
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

func normalizeRestoreSessionsMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off", "no", "false", "0":
		return "off"
	case "on", "yes", "true", "1":
		return "on"
	default:
		return "auto"
	}
}

func parseAutoSortRecentInterval(raw string) time.Duration {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "0" || value == "off" || value == "no" || value == "false" {
		return 0
	}
	if value == "1" || value == "on" || value == "yes" || value == "true" {
		return 24 * time.Hour
	}
	interval, err := config.ParseRelativeDuration(value)
	if err != nil {
		return 0
	}
	return interval
}

func parseHeatRecentInterval(raw string) time.Duration {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "1" || value == "on" || value == "yes" || value == "true" {
		return time.Hour
	}
	interval, err := config.ParseRelativeDuration(value)
	if err != nil {
		return time.Hour
	}
	return interval
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

var configOptionNames = []string{
	"@session-sidebar-key",
	"@session-sidebar-width",
	"@session-sidebar-project-roots",
	"@session-sidebar-close-after-switch",
	"@session-sidebar-heat-colors",
	"@session-sidebar-heat-half-life-hours",
	"@session-sidebar-heat-stale-hours",
	"@session-sidebar-heat-refresh-seconds",
	"@session-sidebar-heat-recent",
	"@session-sidebar-heat-max-highlighted",
	"@session-sidebar-activity-debug-log",
	"@session-sidebar-agent-attention",
	"@session-sidebar-agent-attention-animation",
	"@session-sidebar-auto-sort-recent",
	"@session-sidebar-restore-sessions",
	"@session-sidebar-continuum-grace-seconds",
	"@session-sidebar-color-scheme",
	"@session-sidebar-metadata-subline",
	"@session-sidebar-metadata-inactive",
}

func tmuxConfigQueryFormat() string {
	parts := make([]string, 0, len(configOptionNames))
	for _, name := range configOptionNames {
		parts = append(parts, "#{"+name+"}")
	}
	return strings.Join(parts, configQuerySeparator)
}

func (c Client) loadOptionsMap(ctx context.Context) (map[string]string, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdDisplayMessage, "-p", tmuxConfigQueryFormat()})
	if err == nil {
		values := strings.Split(strings.TrimRight(result.Stdout, "\n"), configQuerySeparator)
		if len(values) == len(configOptionNames) {
			opts := make(map[string]string, len(configOptionNames))
			for i, name := range configOptionNames {
				opts[name] = values[i]
			}
			return opts, nil
		}
	}

	// Fallback for malformed/unsupported tmux output paths (for example,
	// narrow test doubles): preserve the legacy raw per-option semantics.
	opts := make(map[string]string, len(configOptionNames))
	for _, name := range configOptionNames {
		value, err := c.loadOptionValue(ctx, name)
		if err != nil {
			return nil, err
		}
		opts[name] = value
	}
	return opts, nil
}

func (c Client) loadOptionValue(ctx context.Context, name string) (string, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdShowOptions, "-gvq", name})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
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

func concreteTmuxTarget(target string) bool {
	target = strings.TrimSpace(target)
	return strings.HasPrefix(target, "@") || strings.HasPrefix(target, "%") || strings.HasPrefix(target, "/dev/")
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

func (e tmuxError) Is(target error) bool {
	return target == ports.ErrMultiplexerTargetGone && tmuxTargetGoneMessage(e.result.Stderr+e.result.Stdout)
}

func wrapTmuxError(result ports.Result, err error) error {
	if err == nil {
		return nil
	}
	return tmuxError{result: result, err: err}
}

func isTmuxTargetGone(err error) bool {
	return errors.Is(err, ports.ErrMultiplexerTargetGone)
}

func tmuxTargetGoneMessage(output string) bool {
	message := strings.ToLower(output)
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

func sidebarLayoutCleanupCommand(windowID string, paneID string) string {
	script := "window=$1; pane=$2; hidden_option=$3; " +
		"for _ in 1 2 3 4 5 6 7 8 9 10 " +
		"11 12 13 14 15 16 17 18 19 20 " +
		"21 22 23 24 25 26 27 28 29 30 " +
		"31 32 33 34 35 36 37 38 39 40 " +
		"41 42 43 44 45 46 47 48 49 50; do " +
		"tmux list-panes -t \"$window\" -F '" + escapedFormatPaneID + "' 2>/dev/null | grep -Fxq \"$pane\" || break; " +
		"sleep 0.05; " +
		"done; " +
		"tmux list-panes -t \"$window\" -F '" + escapedFormatPaneID + "' 2>/dev/null | grep -Fxq \"$pane\" && exit 0; " +
		"tmux set-option -wu -t \"$window\" \"$hidden_option\" >/dev/null 2>&1 || true"
	return "sh -c " + shellQuote(script) + " sh " + shellQuote(windowID) + " " + shellQuote(paneID) + " " + shellQuote(optionSidebarWindowLayout)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
