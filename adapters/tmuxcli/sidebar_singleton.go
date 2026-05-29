package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func (c Client) FindSingletonSidebar(ctx context.Context) (ports.PaneRef, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", formatPaneID + "\t" + formatWindowID + "\t#{pane_dead}"})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	output := strings.TrimSpace(result.Stdout)
	if err := c.cleanupDeadPaneRefs(ctx, output); err != nil {
		return ports.PaneRef{}, err
	}
	return parseOptionalPaneRef(filterLivePaneRefs(output))
}

func (c Client) EnsureSingletonSidebar(ctx context.Context, command []string) (ports.PaneRef, error) {
	if len(command) == 0 {
		return ports.PaneRef{}, fmt.Errorf("missing singleton sidebar command")
	}
	existing, err := c.FindSingletonSidebar(ctx)
	if err != nil || existing.PaneID != "" {
		return existing, err
	}
	var args []string
	if err := c.parkingSessionExists(ctx); err == nil {
		args = []string{cmdNewWindow, "-d", "-t", singletonSidebarSessionName + ":", "-n", singletonSidebarWindowName, "-P", "-F", formatPaneID + "\t" + formatWindowID}
	} else if isTmuxMissingSessionResult(err) {
		args = []string{cmdNewSession, "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-P", "-F", formatPaneID + "\t" + formatWindowID}
	} else {
		return ports.PaneRef{}, err
	}
	args = append(args, command...)
	result, err := c.Process.Exec(ctx, tmuxBinary, args)
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	ref, err := parsePaneRef(strings.TrimRight(result.Stdout, "\r\n"))
	if err != nil {
		return ports.PaneRef{}, err
	}
	if err := c.markSidebarPane(ctx, ref.PaneID); err != nil {
		if cleanupErr := c.killPane(ctx, ref.PaneID); cleanupErr != nil {
			return ports.PaneRef{}, errors.Join(err, fmt.Errorf("cleanup unmarked singleton sidebar pane %s: %w", ref.PaneID, cleanupErr))
		}
		return ports.PaneRef{}, err
	}
	return ref, nil
}

type sidebarAttachFocus int

const (
	sidebarAttachFocusSidebar sidebarAttachFocus = iota
	sidebarAttachFocusWorkPaneRight
)

func (c Client) AttachSingletonSidebar(ctx context.Context, clientID string, paneID string, width string) (ports.PaneRef, error) {
	return c.attachSingletonSidebar(ctx, clientID, paneID, width, sidebarAttachFocusSidebar)
}

func (c Client) AttachSingletonSidebarWithoutFocus(ctx context.Context, clientID string, paneID string, width string) (ports.PaneRef, error) {
	return c.attachSingletonSidebar(ctx, clientID, paneID, width, sidebarAttachFocusWorkPaneRight)
}

func (c Client) attachSingletonSidebar(ctx context.Context, clientID string, paneID string, width string, focus sidebarAttachFocus) (ports.PaneRef, error) {
	paneID = strings.TrimSpace(paneID)
	if err := c.requireSidebarPane(ctx, paneID); err != nil {
		return ports.PaneRef{}, err
	}
	windowID, err := c.WindowID(ctx, clientID)
	if err != nil {
		return ports.PaneRef{}, err
	}
	currentWindowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return ports.PaneRef{}, err
	}
	width = strings.TrimSpace(width)
	if width == "" {
		width = "20"
	}
	if currentWindowID == windowID {
		if err := c.resizePaneWidth(ctx, paneID, width); err != nil {
			return ports.PaneRef{}, err
		}
		if err := c.selectAttachedSidebarPane(ctx, paneID, focus); err != nil {
			return ports.PaneRef{}, err
		}
		return ports.PaneRef{PaneID: paneID, WindowID: windowID}, nil
	}
	if err := c.saveTargetWindowLayoutBeforeAttach(ctx, windowID); err != nil {
		return ports.PaneRef{}, err
	}
	if err := c.saveVisibleWindowLayoutIfSidebarManaged(ctx, currentWindowID); err != nil {
		return ports.PaneRef{}, err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdJoinPane, "-hbf", "-d", "-l", width, "-s", paneID, "-t", windowID})
	if err != nil {
		_ = c.ClearSavedWindowLayout(ctx, windowID)
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	if err := c.restoreSourceWindowLayoutAfterMove(ctx, currentWindowID); err != nil {
		_ = c.RestoreWindowLayout(ctx, windowID)
		return ports.PaneRef{}, err
	}
	ref := ports.PaneRef{PaneID: paneID, WindowID: windowID}
	if err := c.markSidebarPane(ctx, ref.PaneID); err != nil {
		_ = c.RestoreWindowLayout(ctx, ref.WindowID)
		return ports.PaneRef{}, err
	}
	if !c.restoreVisibleWindowLayoutAfterMove(ctx, ref.WindowID) {
		if err := c.resizePaneWidth(ctx, ref.PaneID, width); err != nil {
			_ = c.RestoreWindowLayout(ctx, ref.WindowID)
			return ports.PaneRef{}, err
		}
	}
	if err := c.selectAttachedSidebarPane(ctx, ref.PaneID, focus); err != nil {
		_ = c.RestoreWindowLayout(ctx, ref.WindowID)
		return ports.PaneRef{}, err
	}
	return ref, nil
}

func (c Client) selectAttachedSidebarPane(ctx context.Context, paneID string, focus sidebarAttachFocus) error {
	switch focus {
	case sidebarAttachFocusWorkPaneRight:
		return c.selectPaneRightOf(ctx, paneID)
	default:
		return c.selectPane(ctx, paneID)
	}
}

func (c Client) AttachSingletonSidebarAndSwitchClient(ctx context.Context, clientID string, sessionName string, paneID string, width string) error {
	paneID = strings.TrimSpace(paneID)
	if err := c.requireSidebarPane(ctx, paneID); err != nil {
		return err
	}
	target := exactSessionWindowTarget(sessionName)
	windowID, err := c.WindowID(ctx, target)
	if err != nil {
		return err
	}
	currentWindowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return err
	}
	width = strings.TrimSpace(width)
	if width == "" {
		width = "20"
	}
	if currentWindowID == windowID {
		if err := c.resizePaneWidth(ctx, paneID, width); err != nil {
			return err
		}
		if err := c.switchClientToExactTarget(ctx, clientID, target); err != nil {
			return err
		}
		return c.selectPaneRightOf(ctx, paneID)
	}
	if err := c.saveTargetWindowLayoutBeforeAttach(ctx, windowID); err != nil {
		return err
	}
	if err := c.saveVisibleWindowLayoutIfSidebarManaged(ctx, currentWindowID); err != nil {
		return err
	}
	args := []string{
		cmdJoinPane, "-hbf", "-d", "-l", width, "-s", paneID, "-t", windowID,
		";", cmdSetOption, "-p", "-t", paneID, optionSidebarPane, "1",
		";", cmdResizePane, "-t", paneID, "-x", width,
		// -R selects the pane to the right of the sidebar, which preserves focus
		// in the work area after the sidebar is moved into the target window.
		";", cmdSelectPane, "-t", paneID, "-R",
	}
	args = append(args, ";")
	args = append(args, switchClientArgs(clientID, target)...)
	result, err := c.Process.Exec(ctx, tmuxBinary, args)
	if err != nil {
		if _, rollbackErr := c.AttachSingletonSidebar(ctx, currentWindowID, paneID, width); rollbackErr != nil {
			return errors.Join(wrapTmuxError(result, err), fmt.Errorf("restore sidebar after failed switch to %q: %w", sessionName, rollbackErr))
		}
		return wrapTmuxError(result, err)
	}
	if err := c.restoreSourceWindowLayoutAfterMove(ctx, currentWindowID); err != nil {
		return err
	}
	_ = c.restoreVisibleWindowLayoutAfterMove(ctx, windowID)
	return nil
}

func (c Client) saveTargetWindowLayoutBeforeAttach(ctx context.Context, windowID string) error {
	return c.captureWindowLayout(ctx, windowID, optionSidebarWindowLayout, true)
}

func (c Client) saveVisibleWindowLayoutIfSidebarManaged(ctx context.Context, windowID string) error {
	layout, err := c.savedWindowLayout(ctx, windowID)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if layout == "" {
		return nil
	}
	return c.SaveVisibleWindowLayout(ctx, windowID)
}

func (c Client) restoreSourceWindowLayoutAfterMove(ctx context.Context, windowID string) error {
	if err := c.RestoreWindowLayout(ctx, windowID); err != nil && !isTmuxTargetGone(err) {
		return err
	}
	return nil
}

func (c Client) restoreHiddenWindowLayoutBestEffort(ctx context.Context, windowID string) {
	if err := c.RestoreWindowLayout(ctx, windowID); err != nil && !isTmuxTargetGone(err) {
		_ = c.ClearSavedWindowLayout(ctx, windowID)
	}
}

func (c Client) restoreVisibleWindowLayoutAfterMove(ctx context.Context, windowID string) bool {
	layout, err := c.windowLayoutOption(ctx, windowID, optionSidebarVisibleWindowLayout)
	if err != nil {
		_ = c.ClearVisibleWindowLayout(ctx, windowID)
		return false
	}
	if layout == "" {
		return false
	}
	_, err = c.Process.Exec(ctx, tmuxBinary, []string{cmdSelectLayout, "-t", windowID, layout})
	if err != nil {
		_ = c.ClearVisibleWindowLayout(ctx, windowID)
		return false
	}
	return true
}

func (c Client) ParkSingletonSidebar(ctx context.Context, paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if err := c.requireSidebarPane(ctx, paneID); err != nil {
		return err
	}
	windowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return err
	}
	if err := c.ensureParkingSession(ctx); err != nil {
		return err
	}
	if err := c.saveVisibleWindowLayoutIfSidebarManaged(ctx, windowID); err != nil {
		return err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdBreakPane, "-d", "-s", paneID, "-t", singletonSidebarSessionName + ":"})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	parkedWindowID, err := c.WindowID(ctx, paneID)
	if err == nil {
		c.cleanupParkingWindows(ctx, parkedWindowID)
	}
	c.restoreHiddenWindowLayoutBestEffort(ctx, windowID)
	return nil
}

func (c Client) cleanupParkingWindows(ctx context.Context, keepWindowID string) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"list-windows", "-t", singletonSidebarSessionName, "-F", formatWindowID})
	if err != nil {
		return
	}
	windows := []string{}
	for windowID := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		windowID = strings.TrimSpace(windowID)
		if windowID != "" {
			windows = append(windows, windowID)
		}
	}
	if len(windows) <= 1 {
		return
	}
	for _, windowID := range windows {
		if windowID == keepWindowID {
			continue
		}
		_, _ = c.Process.Exec(ctx, tmuxBinary, []string{"kill-window", "-t", windowID})
	}
}

func (c Client) ensureParkingSession(ctx context.Context) error {
	if err := c.parkingSessionExists(ctx); err == nil {
		return nil
	} else if !isTmuxMissingSessionResult(err) {
		return err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdNewSession, "-d", "-s", singletonSidebarSessionName, "-n", "sidebar-parked"})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	return nil
}

func (c Client) parkingSessionExists(ctx context.Context) error {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{"has-session", "-t", singletonSidebarSessionName})
	return wrapTmuxError(result, err)
}

func isTmuxMissingSessionResult(err error) bool {
	var tmuxErr tmuxError
	if !errors.As(err, &tmuxErr) {
		return false
	}
	message := strings.ToLower(tmuxErr.result.Stderr + tmuxErr.result.Stdout)
	return strings.Contains(message, "can't find session") || strings.Contains(message, "no such session")
}

func exactSessionWindowTarget(sessionName string) string {
	return "=" + sessionName + ":"
}

func (c Client) requireSidebarPane(ctx context.Context, paneID string) error {
	if paneID == "" {
		return fmt.Errorf("missing singleton sidebar pane")
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdShowOptions, "-pv", "-t", paneID, optionSidebarPane})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	if !parseTmuxBool(result.Stdout) {
		return fmt.Errorf("refusing to move unmarked sidebar pane %s", paneID)
	}
	return nil
}

func (c Client) killPane(ctx context.Context, paneID string) error {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdKillPane, "-t", strings.TrimSpace(paneID)})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	return nil
}

func (c Client) cleanupDeadPaneRefs(ctx context.Context, output string) error {
	for _, paneID := range deadPaneRefIDs(output) {
		if err := c.killPane(ctx, paneID); err != nil && !isTmuxTargetGone(err) {
			return err
		}
	}
	return nil
}

func deadPaneRefIDs(output string) []string {
	var dead []string
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) >= 3 && parseTmuxBool(fields[2]) && strings.TrimSpace(fields[0]) != "" {
			dead = append(dead, strings.TrimSpace(fields[0]))
		}
	}
	return dead
}

func filterLivePaneRefs(output string) string {
	var live []string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 && parseTmuxBool(fields[2]) {
			continue
		}
		live = append(live, line)
	}
	return strings.Join(live, "\n")
}

func parseOptionalPaneRef(output string) (ports.PaneRef, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ports.PaneRef{}, nil
	}
	lines := make([]string, 0, 2)
	for line := range strings.SplitSeq(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ports.PaneRef{}, nil
	}
	if len(lines) > 1 {
		return ports.PaneRef{}, fmt.Errorf("parseOptionalPaneRef: multiple marked sidebar panes found: %q", trimmed)
	}
	return parsePaneRef(lines[0])
}
