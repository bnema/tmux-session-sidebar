package app

import (
	"context"
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
		return err
	}
	return createOrSwitchProject(ctx, flags["client"], projects.CandidateFromPath(root))
}

func createAdhoc(ctx context.Context, flags map[string]string) error {
	name := strings.TrimSpace(flags["name"])
	if name == "" {
		return commandPrompt(ctx, "Ad-hoc session name", "action create-adhoc --client "+shellQuote(flags["client"])+" --name %%")
	}
	path := flags["source-path"]
	if path == "" {
		path = strings.TrimSpace(mustTmux(ctx, "display-message", "-p", "#{pane_current_path}"))
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
		return commandPrompt(ctx, "Rename session", "action rename --session "+shellQuote(session)+" --name %%")
	}
	_, err := tmux(ctx, "rename-session", "-t", session, newName)
	return err
}

func killSession(ctx context.Context, flags map[string]string) error {
	session := flags["session"]
	if session == "" {
		return fmt.Errorf("missing session")
	}
	if flags["confirmed"] != "yes" {
		exe, _ := os.Executable()
		cmd := shellQuote(exe) + " action kill --session " + shellQuote(session) + " --confirmed yes"
		return confirmBefore(ctx, "Kill session "+session+"?", cmd)
	}
	_, err := tmux(ctx, "kill-session", "-t", session)
	return err
}

func gitRoot(ctx context.Context, path string) (string, error) {
	out, err := runCommand(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func commandPrompt(ctx context.Context, prompt string, command string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	full := shellQuote(exe) + " " + command
	_, err = tmux(ctx, "command-prompt", "-p", prompt, "run-shell "+shellQuote(full))
	return err
}

func confirmBefore(ctx context.Context, prompt string, command string) error {
	_, err := tmux(ctx, "confirm-before", "-p", prompt, "run-shell "+shellQuote(command))
	return err
}

func mustTmux(ctx context.Context, args ...string) string {
	out, _ := tmux(ctx, args...)
	return out
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
