package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

type runtimeRouter struct{}

// NewRouter composes the production command router used by the tmux bootstrap.
func NewRouter() Router { return runtimeRouter{} }

func (runtimeRouter) Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error {
	switch route.Path {
	case "sidebar/toggle":
		return toggleSidebar(ctx, route.Flags)
	case "sidebar/open":
		return openSidebar(ctx, route.Flags)
	case "sidebar/close":
		return closeSidebar(ctx, route.Flags)
	case "ui/run":
		return runUI(ctx, stdout)
	case "action/quick-switch":
		return quickSwitch(ctx, route.Flags)
	case "daemon/serve", "daemon/ensure", "hook/client-attached", "hook/client-detached", "hook/client-session-changed":
		return nil
	default:
		_, _ = fmt.Fprintf(stderr, "%s not implemented yet\n", route.Path)
		return nil
	}
}

func toggleSidebar(ctx context.Context, flags map[string]string) error {
	pane, err := existingSidebarPane(ctx, flags["client"])
	if err != nil {
		return err
	}
	if pane != "" {
		_, err := tmux(ctx, "kill-pane", "-t", pane)
		return err
	}
	return openSidebar(ctx, flags)
}

func openSidebar(ctx context.Context, flags map[string]string) error {
	client := flags["client"]
	width, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-width")
	if err != nil || strings.TrimSpace(width) == "" {
		width = "20"
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"split-window", "-P", "-F", "#{pane_id}", "-hb", "-l", strings.TrimSpace(width)}
	if client != "" {
		args = append(args, "-t", client)
	}
	args = append(args, exe, "ui", "run", "--client", client)
	pane, err := tmux(ctx, args...)
	if err != nil {
		return err
	}
	_, err = tmux(ctx, "set-option", "-p", "-t", strings.TrimSpace(pane), "@session-sidebar-pane", "1")
	return err
}

func closeSidebar(ctx context.Context, flags map[string]string) error {
	pane, err := existingSidebarPane(ctx, flags["client"])
	if err != nil || pane == "" {
		return err
	}
	_, err = tmux(ctx, "kill-pane", "-t", pane)
	return err
}

func existingSidebarPane(ctx context.Context, client string) (string, error) {
	windowTarget := client
	if windowTarget == "" {
		windowTarget = "#{window_id}"
	}
	window, err := tmux(ctx, "display-message", "-p", "-t", windowTarget, "#{window_id}")
	if err != nil {
		return "", err
	}
	out, err := tmux(ctx, "list-panes", "-t", strings.TrimSpace(window), "-F", "#{pane_id}\t#{@session-sidebar-pane}")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0], nil
		}
	}
	return "", nil
}

func runUI(ctx context.Context, stdout io.Writer) error {
	for {
		out, err := tmux(ctx, "list-sessions", "-F", "#{session_name}")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(stdout, "\033[H\033[2Jtmux sessions\n\n")
		for i, name := range visibleSessionNames(out) {
			_, _ = fmt.Fprintf(stdout, "[%d] %s\n", i+1, name)
		}
		_, _ = fmt.Fprint(stdout, "\nq: close\n")
		reader := bufio.NewReader(os.Stdin)
		b, err := reader.ReadByte()
		if err != nil || b == 'q' || b == 27 {
			return nil
		}
	}
}

func quickSwitch(ctx context.Context, flags map[string]string) error {
	slot, err := strconv.Atoi(flags["slot"])
	if err != nil || slot <= 0 {
		return fmt.Errorf("invalid quick-switch slot %q", flags["slot"])
	}
	out, err := tmux(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return err
	}
	names := visibleSessionNames(out)
	if slot > len(names) {
		return nil
	}
	args := []string{"switch-client"}
	if flags["client"] != "" {
		args = append(args, "-c", flags["client"])
	}
	args = append(args, "-t", names[slot-1])
	_, err = tmux(ctx, args...)
	return err
}

func visibleSessionNames(out string) []string {
	var names []string
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		name = strings.TrimSpace(name)
		if name != "" && !sessions.IsNumericName(name) {
			names = append(names, name)
		}
	}
	return names
}

func tmux(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
