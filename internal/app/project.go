package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/core/projects"
	coreruntime "github.com/bnema/tmux-session-sidebar/internal/core/runtime"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func createProject(ctx context.Context, flags map[string]string, stdout io.Writer, sidebar ports.SidebarPort) error {
	projectPath := flags["project-path"]
	if projectPath == "" {
		selected, err := pickProject(ctx, stdout)
		if err != nil || selected == "" {
			return err
		}
		projectPath = selected
	}
	candidate := projects.CandidateFromPath(projectPath)
	return createOrSwitchProject(ctx, flags["client"], candidate, flags["category-id"], sidebar)
}

func projectCandidates(ctx context.Context) ([]projects.Candidate, error) {
	rootsOut, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-project-roots")
	if err != nil {
		return nil, err
	}
	fs := runtimeFilesystem()
	var candidates []projects.Candidate
	for root := range strings.SplitSeq(strings.TrimSpace(rootsOut), ":") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root = os.ExpandEnv(root)
		resolvedRoot, err := fs.ResolvePath(root)
		if err != nil {
			if isMissingDependencyError(err) {
				return nil, err
			}
			continue
		}
		dirs, err := fs.ListImmediateDirs(resolvedRoot)
		if err != nil {
			if isMissingDependencyError(err) {
				return nil, err
			}
			continue
		}
		for _, path := range dirs {
			candidates = append(candidates, projects.CandidateFromPath(path))
		}
	}
	return candidates, nil
}

func pickProject(ctx context.Context, stdout io.Writer) (string, error) {
	candidates, err := projectCandidates(ctx)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(stdout, "No project directories found. Press Enter to close.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		return "", nil
	}
	if fzf, err := exec.LookPath("fzf"); err == nil {
		return pickProjectFZF(ctx, fzf, candidates)
	}
	return pickProjectNumbered(stdout, candidates)
}

func pickProjectFZF(ctx context.Context, fzf string, candidates []projects.Candidate) (string, error) {
	var input strings.Builder
	for _, candidate := range candidates {
		parent := filepath.Dir(candidate.Path)
		fmt.Fprintf(&input, "%s\t%s\t[%s]\n", candidate.Path, filepath.Base(candidate.Path), parent)
	}
	cmd := exec.CommandContext(ctx, fzf, "--delimiter=\t", "--with-nth=2,3", "--prompt=project> ", "--header=Enter: create session  Esc: cancel", "--height=100%")
	cmd.Stdin = strings.NewReader(input.String())
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			switch exitErr.ExitCode() {
			case 1, 130:
				return "", nil
			}
		}
		return "", err
	}
	selection := strings.TrimSpace(string(out))
	if selection == "" {
		return "", nil
	}
	return strings.Split(selection, "\t")[0], nil
}

func pickProjectNumbered(stdout io.Writer, candidates []projects.Candidate) (string, error) {
	_, _ = fmt.Fprintln(stdout, "Project sessions")
	_, _ = fmt.Fprintln(stdout)
	for i, candidate := range candidates {
		_, _ = fmt.Fprintf(stdout, "%2d  %-24s %s\n", i+1, candidate.SessionName, candidate.Path)
	}
	_, _ = fmt.Fprint(stdout, "\nSelect project number (Enter to cancel): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	index, err := strconv.Atoi(line)
	if err != nil || index < 1 || index > len(candidates) {
		return "", fmt.Errorf("invalid project selection %q", line)
	}
	return candidates[index-1].Path, nil
}

func createOrSwitchProject(ctx context.Context, client string, candidate projects.Candidate, categoryID string, sidebar ports.SidebarPort) error {
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	plan := coreruntime.ProjectSessionDecision(existing, candidate)
	if !plan.Create {
		return switchClient(ctx, client, plan.SessionName, sidebar)
	}
	if err := withPersistedSessionDuringTmuxAction(ctx, plan.SessionName, plan.Metadata(), func() error {
		return runtimeService().CreateDetachedProjectSession(ctx, existing, plan)
	}); err != nil {
		return err
	}
	if err := saveCreatedSessionCategory(ctx, plan.SessionName, categoryID); err != nil {
		return err
	}
	return switchClient(ctx, client, plan.SessionName, sidebar)
}

func switchClient(ctx context.Context, client string, sessionName string, sidebar ports.SidebarPort) error {
	if err := sessions.ValidateName(sessionName); err != nil {
		return err
	}
	if client != "" && sidebar != nil {
		shouldFollow, err := sidebarShouldBeVisibleForClient(ctx, client)
		if err != nil {
			return err
		}
		if shouldFollow {
			if closeAfterSwitch(ctx, sidebar) {
				output, err := tmux(ctx, appendSwitchClientArgs(client, sessionName)...)
				if err != nil {
					return tmuxCommandError("switch client session", output, err)
				}
				return closeSidebar(ctx, client, sidebar)
			}
			target := exactSessionWindowTarget(sessionName)
			targetSidebar, err := sidebar.FindSidebarPane(ctx, target)
			if err != nil {
				return err
			}
			if targetSidebar.PaneID != "" {
				output, err := tmux(ctx, appendSwitchClientArgs(client, sessionName)...)
				if err != nil {
					return tmuxCommandError("switch client session", output, err)
				}
				return saveSidebarVisibility(ctx, true, client)
			}
			mover, ok := sidebar.(ports.SidebarSwitchPort)
			if !ok {
				return fmt.Errorf("owner-scoped sidebar switch requires atomic tmux move+switch support")
			}
			ownerPane, err := sidebar.FindSidebarPaneForClient(ctx, client)
			if err != nil {
				return err
			}
			cfg := loadSidebarConfig(ctx)
			if ownerPane.PaneID == "" {
				ownerPane, err = ensureSidebarPaneForClient(ctx, client, sidebar)
				if err != nil {
					return err
				}
			}
			if err := mover.AttachSidebarForClientAndSwitchClient(ctx, client, sessionName, ownerPane.PaneID, cfg.Width); err != nil {
				return fmt.Errorf("preposition sidebar and switch to %q: %w", sessionName, err)
			}
			return saveSidebarVisibility(ctx, true, client)
		}
	}
	output, err := tmux(ctx, appendSwitchClientArgs(client, sessionName)...)
	if err != nil {
		return tmuxCommandError("switch client session", output, err)
	}
	return nil
}

func appendSwitchClientArgs(client string, sessionName string) []string {
	args := []string{"switch-client"}
	if client != "" {
		args = append(args, "-c", client)
	}
	return append(args, "-t", exactSessionWindowTarget(sessionName))
}

func exactSessionWindowTarget(sessionName string) string {
	return "=" + sessionName + ":"
}

func loadSessionViews(ctx context.Context) ([]sessions.View, error) {
	return runtimeService().SessionViews(ctx)
}

func runtimeService() *coreruntime.Service {
	return runtimeServiceWithStore(nil)
}

func runtimeServiceWithStore(store ports.StateStorePort) *coreruntime.Service {
	multiplexer := runtimeMultiplexer()
	return coreruntime.NewService(multiplexer, multiplexer, multiplexer, store).WithMetadata(multiplexer)
}
