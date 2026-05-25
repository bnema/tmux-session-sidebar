package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func (c Client) FindSingletonSidebar(ctx context.Context) (ports.PaneRef, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-a", "-f", "#{==:#{" + optionSidebarPane + "},1}", "-F", formatPaneID + "\t" + formatWindowID})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	return parseOptionalPaneRef(strings.TrimSpace(result.Stdout))
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
		return ports.PaneRef{}, err
	}
	return ref, nil
}

func (c Client) AttachSingletonSidebar(ctx context.Context, clientID string, paneID string, width string) (ports.PaneRef, error) {
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
		return ports.PaneRef{PaneID: paneID, WindowID: windowID}, nil
	}
	if err := c.SaveWindowLayout(ctx, windowID); err != nil {
		return ports.PaneRef{}, err
	}
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdJoinPane, "-hbf", "-l", width, "-s", paneID, "-t", windowID})
	if err != nil {
		return ports.PaneRef{}, wrapTmuxError(result, err)
	}
	ref := ports.PaneRef{PaneID: paneID, WindowID: windowID}
	if err := c.markSidebarPane(ctx, ref.PaneID); err != nil {
		_ = c.RestoreWindowLayout(ctx, ref.WindowID)
		return ports.PaneRef{}, err
	}
	if err := c.resizePaneWidth(ctx, ref.PaneID, width); err != nil {
		_ = c.RestoreWindowLayout(ctx, ref.WindowID)
		return ports.PaneRef{}, err
	}
	return ref, nil
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
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdBreakPane, "-d", "-s", paneID, "-t", singletonSidebarSessionName + ":"})
	if err != nil {
		return wrapTmuxError(result, err)
	}
	parkedWindowID, err := c.WindowID(ctx, paneID)
	if err == nil {
		c.cleanupParkingWindows(ctx, parkedWindowID)
	}
	_ = c.RestoreWindowLayout(ctx, windowID)
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

func parseOptionalPaneRef(output string) (ports.PaneRef, error) {
	if output == "" {
		return ports.PaneRef{}, nil
	}
	line, _, _ := strings.Cut(output, "\n")
	return parsePaneRef(strings.TrimSpace(line))
}
