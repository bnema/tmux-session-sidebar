package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/projects"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func createCurrentGitProject(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
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
	return createOrSwitchProject(ctx, flags["client"], projects.CandidateFromPath(root), sidebar)
}

func createAdhoc(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	path := flags["source-path"]
	if path == "" {
		out, err := tmux(ctx, "display-message", "-p", "#{pane_current_path}")
		if err != nil {
			return fmt.Errorf("get current pane path: %w", err)
		}
		path = strings.TrimSpace(out)
	}
	name := strings.TrimSpace(flags["name"])
	if name == "" {
		name = projects.NormalizeSessionName(filepath.Base(filepath.Clean(path)))
	}
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	previousState, err := snapshotSidebarState(ctx)
	if err != nil {
		return err
	}
	metadata := ports.SessionMetadata{Kind: "adhoc", LastPath: path}
	if err := saveSessionMetadata(ctx, name, metadata); err != nil {
		return err
	}
	return withSidebarFollow(ctx, flags["client"], sidebar, func() error {
		if err := runtimeService().CreateAdhocSession(ctx, flags["client"], existing, name, path); err != nil {
			if liveSessionExists(ctx, name) {
				return err
			}
			rollbackPersistedState(ctx, previousState)
			return err
		}
		return nil
	})
}

func renameSession(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
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
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	previousState, err := snapshotSidebarState(ctx)
	if err != nil {
		return err
	}
	if err := renamePersistedSession(ctx, session, newName); err != nil {
		return err
	}
	if err := runtimeService().RenameSession(ctx, existing, session, newName); err != nil {
		rollbackPersistedState(ctx, previousState)
		return err
	}
	if err := refreshSidebar(ctx, flags["client"], sidebar); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: sidebar refresh failed after rename: %v\n", err)
	}
	return nil
}

func killSession(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
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
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	previousState, err := snapshotSidebarState(ctx)
	if err != nil {
		return err
	}
	if err := removePersistedSession(ctx, session); err != nil {
		return err
	}
	if err := runtimeService().KillSession(ctx, existing, session); err != nil {
		rollbackPersistedState(ctx, previousState)
		return err
	}
	if err := refreshSidebar(ctx, flags["client"], sidebar); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: sidebar refresh failed after kill: %v\n", err)
	}
	return nil
}

func rollbackPersistedState(ctx context.Context, previous ports.PersistedState) {
	_ = restoreSidebarState(ctx, previous)
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

func refreshSidebar(ctx context.Context, client string, sidebar ports.TmuxSidebarPort) error {
	return sidebar.RefreshSidebar(ctx, client)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
