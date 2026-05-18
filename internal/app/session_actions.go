package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/projects"
)

func createCurrentGitProject(ctx context.Context, flags map[string]string) error {
	path := flags["source-path"]
	if path == "" {
		var err error
		path, err = tmux(ctx, "display-message", "-p", "#{pane_current_path}")
		if err != nil {
			return err
		}
		path = strings.TrimSpace(path)
	}
	root, err := gitRoot(ctx, path)
	if err != nil || root == "" {
		_, _ = tmux(ctx, "display-message", "tmux-session-sidebar: no git repository found")
		if err != nil {
			return err
		}
		return errors.New("no git repository found")
	}
	return createOrSwitchProject(ctx, flags["client"], projects.CandidateFromPath(root))
}

func createAdhoc(ctx context.Context, flags map[string]string) error {
	name := strings.TrimSpace(flags["name"])
	if name == "" {
		return commandPrompt(ctx, flags["client"], "Ad-hoc session name", "action create-adhoc --client "+shellQuote(flags["client"])+" --name '%%'")
	}
	path := flags["source-path"]
	if path == "" {
		out, err := tmux(ctx, "display-message", "-p", "#{pane_current_path}")
		if err != nil {
			return fmt.Errorf("get current pane path: %w", err)
		}
		path = strings.TrimSpace(out)
	}
	if sessionExists(ctx, name) {
		return switchClient(ctx, flags["client"], name)
	}
	if _, err := tmux(ctx, "new-session", "-d", "-s", name, "-c", path); err != nil {
		return err
	}
	_, _ = tmux(ctx, "set-option", "-t", name, "@session-sidebar-kind", "adhoc")
	return switchClient(ctx, flags["client"], name)
}

func renameSession(ctx context.Context, flags map[string]string) error {
	session := flags["session"]
	newName := strings.TrimSpace(flags["name"])
	if session == "" {
		return fmt.Errorf("missing session")
	}
	if newName == "" {
		command := "action rename --session " + shellQuote(session) + " --name '%%'"
		if flags["client"] != "" {
			command += " --client " + shellQuote(flags["client"])
		}
		return commandPrompt(ctx, flags["client"], "Rename session", command)
	}
	if _, err := tmux(ctx, "rename-session", "-t", "="+session, newName); err != nil {
		return err
	}
	refreshSidebar(ctx, flags["client"])
	return nil
}

func killSession(ctx context.Context, flags map[string]string) error {
	session := flags["session"]
	if session == "" {
		return fmt.Errorf("missing session")
	}
	if flags["confirmed"] != "yes" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable for kill confirmation: %w", err)
		}
		cmd := shellQuote(exe) + " action kill --session " + shellQuote(session) + " --confirmed yes"
		if flags["client"] != "" {
			cmd += " --client " + shellQuote(flags["client"])
		}
		return confirmBefore(ctx, flags["client"], "Kill session "+session+"?", cmd)
	}
	if _, err := tmux(ctx, "kill-session", "-t", "="+session); err != nil {
		return err
	}
	refreshSidebar(ctx, flags["client"])
	return nil
}

func gitRoot(ctx context.Context, path string) (string, error) {
	out, err := runCommand(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func commandPrompt(ctx context.Context, client string, prompt string, command string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	full := shellQuote(exe) + " " + command
	args := []string{"command-prompt"}
	if client != "" {
		args = append(args, "-t", client)
	}
	args = append(args, "-p", prompt, "run-shell "+shellQuote(full))
	_, err = tmux(ctx, args...)
	return err
}

func confirmBefore(ctx context.Context, client string, prompt string, command string) error {
	args := []string{"confirm-before"}
	if client != "" {
		args = append(args, "-t", client)
	}
	args = append(args, "-p", prompt, "run-shell "+shellQuote(command))
	_, err := tmux(ctx, args...)
	return err
}

func refreshSidebar(ctx context.Context, client string) error {
	if client == "" {
		return nil
	}
	pane, err := existingSidebarPane(ctx, client)
	if err != nil || pane == "" {
		return err
	}
	_, err = tmux(ctx, "send-keys", "-t", pane, "F5")
	return err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
