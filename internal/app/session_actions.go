package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/projects"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
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
	return createOrSwitchProject(ctx, flags["client"], projects.CandidateFromPath(root), flags["category-id"], sidebar)
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
	if err := withSidebarFollow(ctx, flags["client"], sidebar, func() error {
		return withPersistedSessionDuringTmuxAction(ctx, name, metadata, func() error {
			return runtimeService().CreateAdhocSession(ctx, flags["client"], existing, name, path)
		})
	}); err != nil {
		return err
	}
	return saveCreatedSessionCategory(ctx, name, flags["category-id"])
}

func saveCreatedSessionCategory(ctx context.Context, name string, categoryID string) error {
	if strings.TrimSpace(categoryID) == "" {
		return nil
	}
	live, err := currentLiveSessionNames(ctx)
	if err != nil {
		return err
	}
	return saveSidebarSessionCategory(ctx, name, categoryID, live)
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
	if err := switchAwayBeforeKillingCurrentSession(ctx, flags["client"], session, existing, sidebar); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: switch away before kill failed: %v\n", err)
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

func switchAwayBeforeKillingCurrentSession(ctx context.Context, client string, target string, existing []sessions.View, sidebar ports.TmuxSidebarPort) error {
	current, err := currentSessionForKill(ctx, client, existing)
	if err != nil {
		return err
	}
	if current != target {
		return nil
	}
	replacement := replacementSessionForKill(existing, target)
	if replacement == "" {
		return nil
	}
	return switchClient(ctx, client, replacement, sidebar)
}

func currentSessionForKill(ctx context.Context, client string, existing []sessions.View) (string, error) {
	if strings.TrimSpace(client) != "" {
		out, err := tmux(ctx, "display-message", "-p", "-t", client, "#{session_name}")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	}
	out, err := tmux(ctx, "display-message", "-p", "#{session_name}")
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	for _, session := range existing {
		if session.Current {
			return session.Name, nil
		}
	}
	return "", nil
}

func replacementSessionForKill(existing []sessions.View, target string) string {
	for _, session := range existing {
		if session.Name != target && session.Visible {
			return session.Name
		}
	}
	for _, session := range existing {
		if session.Name != target {
			return session.Name
		}
	}
	return ""
}

func withPersistedSessionDuringTmuxAction(ctx context.Context, name string, metadata ports.SessionMetadata, action func() error) error {
	if !shouldPersistSessionName(name) {
		return action()
	}
	return withLoadedSidebarState(ctx, func(store scopedStateStore, state *ports.PersistedState) error {
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
	return withLoadedSidebarState(ctx, func(store scopedStateStore, state *ports.PersistedState) error {
		previous := clonePersistedState(*state)
		renameSessionState(state, oldName, newName)
		if err := saveLoadedSidebarState(ctx, store, *state); err != nil {
			return err
		}
		if err := action(); err != nil {
			if !renameLiveStateSucceeded(ctx, oldName, newName) {
				rollbackLoadedSidebarState(ctx, store, previous)
			}
			return err
		}
		return nil
	})
}

func renameLiveStateSucceeded(ctx context.Context, oldName string, newName string) bool {
	return !liveSessionExists(ctx, oldName) && liveSessionExists(ctx, newName)
}

func withRemovedPersistedSessionDuringTmuxAction(ctx context.Context, name string, action func() error) error {
	return withLoadedSidebarState(ctx, func(store scopedStateStore, state *ports.PersistedState) error {
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

func rollbackLoadedSidebarState(ctx context.Context, store scopedStateStore, previous ports.PersistedState) {
	// Best-effort restore; the primary operation error takes precedence over rollback failures.
	if err := saveLoadedSidebarState(ctx, store, previous); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: rollback persisted sidebar state failed: %v\n", err)
	}
}

func gitRoot(ctx context.Context, path string) (string, error) {
	return runtimeGit().RepoRoot(ctx, path)
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
