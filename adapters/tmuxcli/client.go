package tmuxcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

var ErrLayoutUnsupported = errors.New("tmux layout save/restore is not supported by tmuxcli adapter")

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
	return ports.ConfigSnapshot{KeyBinding: key, Width: width, ProjectRoots: splitProjectRoots(roots)}, nil
}

func (c Client) ServerID(ctx context.Context) (string, error) {
	return c.display(ctx, "#{socket_path}")
}

func (c Client) ListSessions(ctx context.Context) ([]ports.TmuxSessionSnapshot, error) {
	result, err := c.Process.Exec(ctx, "tmux", []string{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_windows}\t#{session_attached}"})
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
	result, err := c.Process.Exec(ctx, "tmux", []string{"list-clients", "-F", "#{client_name}\t#{session_id}\t#{window_id}\t#{pane_id}\t#{client_session}"})
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

func (c Client) CurrentPanePath(ctx context.Context, clientID string) (string, error) {
	return c.displayTarget(ctx, clientID, "#{pane_current_path}")
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
	_, err := c.Process.Exec(ctx, "tmux", args)
	return err
}

func (c Client) DisplayMessage(ctx context.Context, clientID string, message string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"display-message", "-c", clientID, message})
	return err
}

func (c Client) CreateSession(ctx context.Context, sessionName string, path string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"new-session", "-d", "-s", sessionName, "-c", path})
	return err
}

func (c Client) RenameSession(ctx context.Context, oldName string, newName string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"rename-session", "-t", "=" + oldName, newName})
	return err
}

func (c Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"kill-session", "-t", "=" + sessionName})
	return err
}

func (c Client) OpenSidebarPane(ctx context.Context, clientID string, width string, command []string) (ports.PaneRef, error) {
	args := []string{"split-window", "-P", "-F", "#{pane_id}\t#{window_id}"}
	if clientID != "" {
		args = append(args, "-t", clientID)
	}
	args = append(args, "-hb", "-l", width)
	args = append(args, command...)
	result, err := c.Process.Exec(ctx, "tmux", args)
	if err != nil {
		return ports.PaneRef{}, err
	}
	out := strings.TrimRight(result.Stdout, "\r\n")
	if out == "" {
		return ports.PaneRef{}, fmt.Errorf("open sidebar pane: empty tmux output")
	}
	fields := strings.Split(out, "\t")
	if len(fields) == 0 || fields[0] == "" {
		return ports.PaneRef{}, fmt.Errorf("open sidebar pane: malformed tmux output %q", out)
	}
	ref := ports.PaneRef{PaneID: fields[0]}
	if len(fields) > 1 {
		ref.WindowID = fields[1]
	}
	return ref, nil
}

func (c Client) ClosePane(ctx context.Context, paneID string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"kill-pane", "-t", paneID})
	return err
}

func (c Client) SaveWindowLayout(ctx context.Context, windowID string) error {
	return ErrLayoutUnsupported
}
func (c Client) RestoreWindowLayout(ctx context.Context, windowID string) error {
	return ErrLayoutUnsupported
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

func (c Client) display(ctx context.Context, format string) (string, error) {
	result, err := c.Process.Exec(ctx, "tmux", []string{"display-message", "-p", format})
	return strings.TrimSpace(result.Stdout), err
}

func (c Client) displayTarget(ctx context.Context, target string, format string) (string, error) {
	result, err := c.Process.Exec(ctx, "tmux", []string{"display-message", "-p", "-t", target, format})
	return strings.TrimSpace(result.Stdout), err
}
