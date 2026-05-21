package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/core/projects"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func createCurrentGitProject(ctx context.Context, flags map[string]string, sidebar ports.TmuxSidebarPort) error {
	path := flags["source-path"]
	if path == "" {
		var err error
		path, err = currentPanePathForAction(ctx, flags["client"])
		if err != nil {
			return err
		}
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
		var err error
		path, err = currentPanePathForAction(ctx, flags["client"])
		if err != nil {
			return fmt.Errorf("get current pane path: %w", err)
		}
	}
	name := strings.TrimSpace(flags["name"])
	if name == "" {
		name = projects.NormalizeSessionName(filepath.Base(filepath.Clean(path)))
	}
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	metadata := ports.SessionMetadata{Kind: "adhoc", LastPath: path}
	return withSidebarFollow(ctx, flags["client"], sidebar, func() error {
		return withPersistedSessionDuringTmuxAction(ctx, name, metadata, func() error {
			return runtimeService().CreateAdhocSession(ctx, flags["client"], existing, name, path)
		})
	})
}

func currentPanePathForAction(ctx context.Context, client string) (string, error) {
	client = strings.TrimSpace(client)
	if client == "" {
		out, err := tmux(ctx, "display-message", "-p", "#{pane_current_path}")
		return strings.TrimSpace(out), err
	}
	windowID, err := tmux(ctx, "display-message", "-p", "-t", client, "#{window_id}")
	if err == nil && strings.TrimSpace(windowID) != "" {
		out, listErr := tmux(ctx, "list-panes", "-t", strings.TrimSpace(windowID), "-F", "#{pane_id}\t#{pane_active}\t#{pane_last}\t#{@session-sidebar-pane}\t#{pane_current_path}")
		if listErr == nil {
			var firstWorkPanePath string
			var lastWorkPanePath string
			for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
				fields := strings.SplitN(line, "\t", 5)
				if len(fields) < 5 || strings.TrimSpace(fields[3]) == "1" {
					continue
				}
				path := strings.TrimSpace(fields[4])
				if path == "" {
					continue
				}
				if firstWorkPanePath == "" {
					firstWorkPanePath = path
				}
				if strings.TrimSpace(fields[1]) == "1" {
					return path, nil
				}
				if strings.TrimSpace(fields[2]) == "1" {
					lastWorkPanePath = path
				}
			}
			if lastWorkPanePath != "" {
				return lastWorkPanePath, nil
			}
			if firstWorkPanePath != "" {
				return firstWorkPanePath, nil
			}
		}
	}
	out, err := tmux(ctx, "display-message", "-p", "-t", client, "#{pane_current_path}")
	return strings.TrimSpace(out), err
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
	if err := withRenamedPersistedSessionDuringTmuxAction(ctx, session, newName, func() error {
		return runtimeService().RenameSession(ctx, existing, session, newName)
	}); err != nil {
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
	if err := withRemovedPersistedSessionDuringTmuxAction(ctx, session, func() error {
		return runtimeService().KillSession(ctx, existing, session)
	}); err != nil {
		return err
	}
	if err := refreshSidebar(ctx, flags["client"], sidebar); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: sidebar refresh failed after kill: %v\n", err)
	}
	return nil
}

func withPersistedSessionDuringTmuxAction(ctx context.Context, name string, metadata ports.SessionMetadata, action func() error) error {
	if !shouldPersistSessionName(name) {
		return action()
	}
	return withLoadedSidebarState(ctx, func(store storefs.Store, state *ports.PersistedState) error {
		previous := clonePersistedState(*state)
		saveSessionMetadataState(state, name, metadata)
		if err := saveLoadedSidebarState(ctx, store, *state); err != nil {
			return err
		}
		if err := action(); err != nil {
			if liveSessionExists(ctx, name) {
				return err
			}
			rollbackLoadedSidebarState(ctx, store, previous)
			return err
		}
		return nil
	})
}

func withRenamedPersistedSessionDuringTmuxAction(ctx context.Context, oldName string, newName string, action func() error) error {
	return withLoadedSidebarState(ctx, func(store storefs.Store, state *ports.PersistedState) error {
		previous := clonePersistedState(*state)
		renameSessionState(state, oldName, newName)
		if err := saveLoadedSidebarState(ctx, store, *state); err != nil {
			return err
		}
		if err := action(); err != nil {
			if !liveSessionExists(ctx, newName) {
				rollbackLoadedSidebarState(ctx, store, previous)
			}
			return err
		}
		return nil
	})
}

func withRemovedPersistedSessionDuringTmuxAction(ctx context.Context, name string, action func() error) error {
	return withLoadedSidebarState(ctx, func(store storefs.Store, state *ports.PersistedState) error {
		previous := clonePersistedState(*state)
		removeSessionState(state, name)
		if err := saveLoadedSidebarState(ctx, store, *state); err != nil {
			return err
		}
		if err := action(); err != nil {
			if liveSessionExists(ctx, name) {
				rollbackLoadedSidebarState(ctx, store, previous)
			}
			return err
		}
		return nil
	})
}

func rollbackLoadedSidebarState(ctx context.Context, store storefs.Store, previous ports.PersistedState) {
	// Best-effort restore; the primary operation error takes precedence over rollback failures.
	if err := saveLoadedSidebarState(ctx, store, previous); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: rollback persisted sidebar state failed: %v\n", err)
	}
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
