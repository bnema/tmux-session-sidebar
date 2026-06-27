package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func (c Client) FindSidebarPaneForClient(ctx context.Context, ownerClientID string) (ports.PaneRef, error) {
	ownerClientID = strings.TrimSpace(ownerClientID)
	if ownerClientID == "" {
		return ports.PaneRef{}, fmt.Errorf("missing sidebar owner client")
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-f", "#{&&:#{==:#{" + optionSidebarPane + "},1},#{==:#{" + optionSidebarOwnerClient + "}," + ownerClientID + "}}", "-F", formatPaneID + "\t" + formatWindowID + "\t#{pane_dead}"})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	output := strings.TrimSpace(result.Stdout)
	if err := c.cleanupDeadPaneRefs(ctx, output); err != nil {
		return ports.PaneRef{}, err
	}
	return parseOptionalPaneRef(filterLivePaneRefs(output))
}

func (c Client) FindSingletonSidebar(ctx context.Context) (ports.PaneRef, error) {
	panes, err := c.listMarkedSidebarPanes(ctx)
	if err != nil {
		return ports.PaneRef{}, err
	}
	if len(panes) == 0 {
		return ports.PaneRef{}, nil
	}
	if len(panes) > 1 {
		return ports.PaneRef{}, fmt.Errorf("FindSingletonSidebar: multiple marked sidebar panes found: %v", panes)
	}
	return panes[0], nil
}

func (c Client) listMarkedSidebarPanes(ctx context.Context) ([]ports.PaneRef, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", formatPaneID + "\t" + formatWindowID + "\t#{pane_dead}"})
	if err != nil {
		return nil, wrapTmuxError(result, err)
	}
	output := strings.TrimSpace(result.Stdout)
	if err := c.cleanupDeadPaneRefs(ctx, output); err != nil {
		return nil, err
	}
	return parsePaneRefs(filterLivePaneRefs(output))
}

func (c Client) EnsureSidebarForClient(ctx context.Context, ownerClientID string, command []string) (ports.PaneRef, error) {
	ownerClientID = strings.TrimSpace(ownerClientID)
	if ownerClientID == "" {
		return ports.PaneRef{}, fmt.Errorf("missing sidebar owner client")
	}
	if len(command) == 0 {
		return ports.PaneRef{}, fmt.Errorf("missing singleton sidebar command")
	}
	existing, err := c.FindSidebarPaneForClient(ctx, ownerClientID)
	if err != nil || existing.PaneID != "" {
		return existing, err
	}
	ref, err := c.createParkedSidebar(ctx, command)
	if err != nil {
		return ports.PaneRef{}, err
	}
	if err := c.markSidebarPaneForClient(ctx, ref.PaneID, ownerClientID); err != nil {
		if cleanupErr := c.killPane(ctx, ref.PaneID); cleanupErr != nil {
			return ports.PaneRef{}, errors.Join(err, fmt.Errorf("cleanup unmarked singleton sidebar pane %s: %w", ref.PaneID, cleanupErr))
		}
		return ports.PaneRef{}, err
	}
	return ref, nil
}

func (c Client) EnsureSingletonSidebar(ctx context.Context, command []string) (ports.PaneRef, error) {
	if len(command) == 0 {
		return ports.PaneRef{}, fmt.Errorf("missing singleton sidebar command")
	}
	existing, err := c.FindSingletonSidebar(ctx)
	if err != nil || existing.PaneID != "" {
		return existing, err
	}
	ref, err := c.createParkedSidebar(ctx, command)
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

func (c Client) createParkedSidebar(ctx context.Context, command []string) (ports.PaneRef, error) {
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
	return parsePaneRef(strings.TrimRight(result.Stdout, "\r\n"))
}

type sidebarAttachFocus int

const (
	sidebarAttachFocusSidebar sidebarAttachFocus = iota
	sidebarAttachFocusWorkPaneRight
)

func (c Client) AttachSidebarForClient(ctx context.Context, ownerClientID string, targetID string, paneID string, width string) (ports.PaneRef, error) {
	if err := c.markSidebarPaneForClient(ctx, paneID, ownerClientID); err != nil {
		return ports.PaneRef{}, err
	}
	return c.attachSingletonSidebar(ctx, targetID, paneID, width, sidebarAttachFocusSidebar, true)
}

func (c Client) AttachSingletonSidebar(ctx context.Context, clientID string, paneID string, width string) (ports.PaneRef, error) {
	return c.attachSingletonSidebar(ctx, clientID, paneID, width, sidebarAttachFocusSidebar, true)
}

func (c Client) AttachSidebarForClientWithoutFocus(ctx context.Context, ownerClientID string, targetID string, paneID string, width string) (ports.PaneRef, error) {
	if err := c.markSidebarPaneForClient(ctx, paneID, ownerClientID); err != nil {
		return ports.PaneRef{}, err
	}
	return c.attachSingletonSidebar(ctx, targetID, paneID, width, sidebarAttachFocusWorkPaneRight, true)
}

func (c Client) AttachSingletonSidebarWithoutFocus(ctx context.Context, clientID string, paneID string, width string) (ports.PaneRef, error) {
	return c.attachSingletonSidebar(ctx, clientID, paneID, width, sidebarAttachFocusWorkPaneRight, true)
}

func (c Client) attachSingletonSidebar(ctx context.Context, targetID string, paneID string, width string, focus sidebarAttachFocus, clearSourceWindowLayout bool) (ports.PaneRef, error) {
	paneID = strings.TrimSpace(paneID)
	if err := c.requireSidebarPane(ctx, paneID); err != nil {
		return ports.PaneRef{}, err
	}
	windowID, err := c.WindowID(ctx, targetID)
	if err != nil {
		return ports.PaneRef{}, err
	}
	currentWindowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return ports.PaneRef{}, err
	}
	width = strings.TrimSpace(width)
	if width == "" {
		width = "30"
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
	var sourceCloseWeights []sidebarWorkWeight
	if clearSourceWindowLayout {
		sourceCloseWeights, _, _ = c.captureSidebarWorkWeights(ctx, currentWindowID, paneID, "", sidebarWorkWeightByGroupSpan)
	}
	// Save target hidden layout for rollback if the multi-step attach fails.
	if err := c.saveTargetWindowLayoutBeforeAttach(ctx, windowID); err != nil {
		return ports.PaneRef{}, err
	}
	if err := c.joinSidebarPane(ctx, paneID, windowID, width); err != nil {
		_ = c.ClearSavedWindowLayout(ctx, windowID)
		return ports.PaneRef{}, err
	}
	ref := ports.PaneRef{PaneID: paneID, WindowID: windowID}
	if err := c.markSidebarPane(ctx, ref.PaneID); err != nil {
		c.rollbackAttachedSidebarAfterJoin(ctx, paneID, width, currentWindowID, ref.WindowID, sourceCloseWeights)
		return ports.PaneRef{}, err
	}
	// The live tmux layout is authoritative after join-pane. Only set the
	// explicit sidebar width; tmux handles work-pane layout natively.
	if err := c.resizePaneWidth(ctx, ref.PaneID, width); err != nil {
		c.rollbackAttachedSidebarAfterJoin(ctx, paneID, width, currentWindowID, ref.WindowID, sourceCloseWeights)
		return ports.PaneRef{}, err
	}
	if err := c.selectAttachedSidebarPane(ctx, ref.PaneID, focus); err != nil {
		c.rollbackAttachedSidebarAfterJoin(ctx, paneID, width, currentWindowID, ref.WindowID, sourceCloseWeights)
		return ports.PaneRef{}, err
	}
	if clearSourceWindowLayout {
		c.rebalanceSourceWindowAfterSidebarMoveBestEffort(ctx, currentWindowID, sourceCloseWeights)
		c.captureAttachedSidebarWidthBaselineBestEffort(ctx, ref.WindowID, ref.PaneID, width)
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

func (c Client) joinSidebarPane(ctx context.Context, paneID string, windowID string, width string) error {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdJoinPane, "-hbf", "-d", "-l", width, "-s", paneID, "-t", windowID})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	return nil
}

func (c Client) rollbackAttachedSidebarAfterJoin(ctx context.Context, paneID string, width string, currentWindowID string, targetWindowID string, sourceCloseWeights []sidebarWorkWeight) {
	c.rebalanceSourceWindowAfterSidebarMoveBestEffort(ctx, currentWindowID, sourceCloseWeights)
	if err := c.joinSidebarPane(ctx, paneID, currentWindowID, width); err != nil {
		return
	}
	_ = c.RestoreWindowLayout(ctx, targetWindowID)
}

func (c Client) AttachSingletonSidebarAndSwitchClient(ctx context.Context, clientID string, sessionName string, paneID string, width string) error {
	return c.AttachSidebarForClientAndSwitchClient(ctx, clientID, sessionName, paneID, width)
}

func (c Client) AttachSidebarForClientAndSwitchClient(ctx context.Context, clientID string, sessionName string, paneID string, width string) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("missing sidebar owner client")
	}
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
		width = "30"
	}
	if currentWindowID == windowID {
		preSyncSidebarWidth, _ := c.displayTarget(ctx, paneID, "#{pane_width}")
		preSyncWeights, _, _ := c.captureSidebarWorkWeights(ctx, windowID, paneID, preSyncSidebarWidth, sidebarWorkWeightByGroupWidth)
		if err := c.SyncAttachedSidebarWidth(ctx, windowID, paneID, width, ports.SidebarResizeOptions{}); err != nil {
			return err
		}
		if err := c.switchMarkAndSelectSidebarPane(ctx, clientID, target, paneID); err != nil {
			c.restoreAttachedSidebarLayoutBestEffort(ctx, windowID, paneID, preSyncSidebarWidth, preSyncWeights)
			return err
		}
		return nil
	}
	sourceCloseWeights, _, _ := c.captureSidebarWorkWeights(ctx, currentWindowID, paneID, "", sidebarWorkWeightByGroupSpan)
	if err := c.saveTargetWindowLayoutBeforeAttach(ctx, windowID); err != nil {
		return err
	}
	args := []string{
		cmdJoinPane, "-hbf", "-d", "-l", width, "-s", paneID, "-t", windowID,
		";", cmdSetOption, "-p", "-t", paneID, optionSidebarPane, "1",
		";", cmdSetOption, "-p", "-t", paneID, optionSidebarOwnerClient, clientID,
		";", cmdResizePane, "-t", paneID, "-x", width,
	}
	args = append(args, ";")
	args = append(args, switchClientArgs(clientID, target)...)
	// Select after switching the client so the visible client lands in the work
	// pane to the right of the sidebar instead of staying focused on the sidebar.
	args = append(args, ";", cmdSelectPane, "-t", paneID, "-R")
	result, err := c.Process.Exec(ctx, tmuxBinary, args)
	if err != nil {
		if c.sidebarPaneMovedAwayFromWindowBestEffort(ctx, paneID, currentWindowID) {
			c.rebalanceSourceWindowAfterSidebarMoveBestEffort(ctx, currentWindowID, sourceCloseWeights)
		}
		if _, rollbackErr := c.attachSingletonSidebar(ctx, currentWindowID, paneID, width, sidebarAttachFocusSidebar, false); rollbackErr != nil {
			return errors.Join(wrapTmuxError(result, err), fmt.Errorf("restore sidebar after failed switch to %q: %w", sessionName, rollbackErr))
		}
		if restoreErr := c.RestoreWindowLayout(ctx, windowID); restoreErr != nil && !isTmuxTargetGone(restoreErr) {
			return errors.Join(wrapTmuxError(result, err), fmt.Errorf("restore target window layout after failed switch to %q: %w", sessionName, restoreErr))
		}
		return wrapTmuxError(result, err)
	}
	c.rebalanceSourceWindowAfterSidebarMoveBestEffort(ctx, currentWindowID, sourceCloseWeights)
	c.captureAttachedSidebarWidthBaselineBestEffort(ctx, windowID, paneID, width)
	return nil
}

func (c Client) restoreAttachedSidebarLayoutBestEffort(ctx context.Context, windowID string, paneID string, sidebarWidth string, weights []sidebarWorkWeight) {
	if strings.TrimSpace(sidebarWidth) != "" {
		_ = c.resizePaneWidth(ctx, paneID, sidebarWidth)
	}
	c.applySidebarWorkWeightsBestEffort(ctx, windowID, paneID, weights, true)
}

func (c Client) switchMarkAndSelectSidebarPane(ctx context.Context, clientID string, target string, paneID string) error {
	args := switchClientArgs(clientID, target)
	args = append(args,
		";", cmdSetOption, "-p", "-t", paneID, optionSidebarPane, "1",
		";", cmdSetOption, "-p", "-t", paneID, optionSidebarOwnerClient, clientID,
		";", cmdSelectPane, "-t", paneID, "-R",
	)
	result, err := c.Process.Exec(ctx, tmuxBinary, args)
	if err != nil {
		return wrapTmuxError(result, err)
	}
	return nil
}

func (c Client) captureAttachedSidebarWidthBaselineBestEffort(ctx context.Context, windowID string, paneID string, width string) {
	_ = c.CaptureAttachedSidebarWidthBaseline(ctx, windowID, paneID, width, ports.SidebarResizeOptions{})
}

func (c Client) sidebarPaneMovedAwayFromWindowBestEffort(ctx context.Context, paneID string, originalWindowID string) bool {
	paneID = strings.TrimSpace(paneID)
	originalWindowID = strings.TrimSpace(originalWindowID)
	if paneID == "" || originalWindowID == "" {
		return false
	}
	windowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return false
	}
	return strings.TrimSpace(windowID) != originalWindowID
}

func (c Client) saveTargetWindowLayoutBeforeAttach(ctx context.Context, windowID string) error {
	return c.captureWindowLayout(ctx, windowID, optionSidebarWindowLayout)
}

func (c Client) rebalanceSourceWindowAfterSidebarMoveBestEffort(ctx context.Context, windowID string, weights []sidebarWorkWeight) {
	c.applySidebarWorkWeightsBestEffort(ctx, windowID, "", weights, false)
	c.clearSourceWindowLayoutBestEffort(ctx, windowID)
}

func (c Client) clearSourceWindowLayoutBestEffort(ctx context.Context, windowID string) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return
	}
	_ = c.ClearSavedWindowLayout(ctx, windowID)
}

func (c Client) ParkSidebarForClient(ctx context.Context, ownerClientID string, paneID string) error {
	if err := c.markSidebarPaneForClient(ctx, paneID, ownerClientID); err != nil {
		return err
	}
	return c.ParkSingletonSidebar(ctx, paneID)
}

func (c Client) ParkAllSidebars(ctx context.Context) error {
	panes, err := c.listMarkedSidebarPanes(ctx)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if err := c.ParkSingletonSidebar(ctx, pane.PaneID); err != nil {
			return fmt.Errorf("park sidebar pane %s: %w", pane.PaneID, err)
		}
	}
	return nil
}

func (c Client) ParkSingletonSidebar(ctx context.Context, paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if err := c.requireSidebarPane(ctx, paneID); err != nil {
		return err
	}
	sourceWindowID, err := c.WindowID(ctx, paneID)
	if err != nil {
		return err
	}
	horizontalCloseWeights, _, _ := c.captureSidebarWorkWeights(ctx, sourceWindowID, paneID, "", sidebarWorkWeightByGroupSpan)
	if err := c.ensureParkingSession(ctx); err != nil {
		return err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdMovePane, "-d", "-s", paneID, "-t", singletonSidebarSessionName + ":"})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	c.applySidebarWorkWeightsBestEffort(ctx, sourceWindowID, "", horizontalCloseWeights, false)
	// Best-effort cleanup of the stale saved hidden-layout option. After
	// move-pane removes the sidebar, tmux natively redistributes the
	// window, making any saved pre-sidebar layout stale unless the close path
	// first rebalances top-level horizontal work-group widths.
	c.clearSourceWindowLayoutBestEffort(ctx, sourceWindowID)

	parkedWindowID, err := c.WindowID(ctx, paneID)
	if err == nil {
		c.cleanupParkingWindows(ctx, parkedWindowID)
	}
	// After move-pane, tmux naturally redistributes remaining panes.
	// The live layout is authoritative — no hidden-layout restore is needed.
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
	keepWindows, ok := c.markedSidebarWindowIDsBestEffort(ctx)
	if !ok {
		return
	}
	if keepWindowID != "" {
		keepWindows[keepWindowID] = true
	}
	for _, windowID := range windows {
		if keepWindows[windowID] {
			continue
		}
		_, _ = c.Process.Exec(ctx, tmuxBinary, []string{"kill-window", "-t", windowID})
	}
}

func (c Client) markedSidebarWindowIDsBestEffort(ctx context.Context) (map[string]bool, bool) {
	windows := map[string]bool{}
	panes, err := c.listMarkedSidebarPanes(ctx)
	if err != nil {
		return nil, false
	}
	for _, pane := range panes {
		windowID := strings.TrimSpace(pane.WindowID)
		if windowID != "" {
			windows[windowID] = true
		}
	}
	return windows, true
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
	refs, err := parsePaneRefs(output)
	if err != nil {
		return ports.PaneRef{}, err
	}
	if len(refs) == 0 {
		return ports.PaneRef{}, nil
	}
	if len(refs) > 1 {
		return ports.PaneRef{}, fmt.Errorf("parseOptionalPaneRef: multiple marked sidebar panes found: %q", strings.TrimSpace(output))
	}
	return refs[0], nil
}

func parsePaneRefs(output string) ([]ports.PaneRef, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, nil
	}
	refs := []ports.PaneRef{}
	for line := range strings.SplitSeq(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ref, err := parsePaneRef(line)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
