package tmuxcli

import (
	"context"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
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
	return ports.ConfigSnapshot{KeyBinding: key, Width: width, ProjectRoots: strings.Split(roots, ":")}, nil
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
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		windows, _ := strconv.Atoi(fields[2])
		attached, _ := strconv.Atoi(fields[3])
		sessions = append(sessions, ports.TmuxSessionSnapshot{ID: fields[0], Name: fields[1], WindowCount: windows, AttachedCount: attached})
	}
	return sessions, nil
}

func (c Client) ListClients(ctx context.Context) ([]ports.TmuxClientSnapshot, error) {
	result, err := c.Process.Exec(ctx, "tmux", []string{"list-clients", "-F", "#{client_name}\t#{session_id}\t#{window_id}\t#{pane_id}"})
	if err != nil {
		return nil, err
	}
	var clients []ports.TmuxClientSnapshot
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		clients = append(clients, ports.TmuxClientSnapshot{ID: fields[0], CurrentSessionID: fields[1], CurrentWindowID: fields[2], CurrentPaneID: fields[3], Attached: true})
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
	width, _ := strconv.Atoi(fields[0])
	height := 0
	if len(fields) > 1 {
		height, _ = strconv.Atoi(fields[1])
	}
	return ports.PaneSize{Width: width, Height: height}, nil
}

func (c Client) SwitchClientSession(ctx context.Context, clientID string, sessionName string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"switch-client", "-c", clientID, "-t", sessionName})
	return err
}

func (c Client) DisplayMessage(ctx context.Context, clientID string, message string) error {
	_, err := c.Process.Exec(ctx, "tmux", []string{"display-message", "-c", clientID, message})
	return err
}

func (c Client) OpenSidebarPane(ctx context.Context, clientID string, width string, command []string) (ports.PaneRef, error) {
	args := []string{"split-window", "-P", "-F", "#{pane_id}\t#{window_id}", "-hb", "-l", width}
	args = append(args, command...)
	result, err := c.Process.Exec(ctx, "tmux", args)
	if err != nil {
		return ports.PaneRef{}, err
	}
	fields := strings.Split(strings.TrimSpace(result.Stdout), "\t")
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

func (c Client) SaveWindowLayout(ctx context.Context, windowID string) error    { return nil }
func (c Client) RestoreWindowLayout(ctx context.Context, windowID string) error { return nil }

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
