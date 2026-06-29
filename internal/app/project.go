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
			ownerPane, err := sidebar.FindSidebarPaneForClient(ctx, client)
			if err != nil {
				return err
			}
			targetRef, err := bestTargetSidebarPane(ctx, sidebar, target, client, ownerPane)
			if err != nil {
				return err
			}
			mover, ok := sidebar.(ports.SidebarSwitchPort)
			if targetRef.PaneID != "" {
				reusableTarget, err := targetSidebarReusableForClient(ctx, targetRef, client)
				if err != nil {
					return err
				}
				if reusableTarget {
					if !ok {
						return fmt.Errorf("target-window sidebar adoption requires atomic tmux move+switch support")
					}
					if err := adoptTargetSidebar(ctx, mover, client, sessionName, targetRef, loadSidebarConfig(ctx)); err != nil {
						return err
					}
					saveSidebarVisibilityAfterCommittedSwitch(ctx, client)
					parkPreviousSidebarAfterAdoption(ctx, sidebar, client, ownerPane, targetRef)
					return nil
				}
				if err := parkNonReusableTargetSidebar(ctx, sidebar, client, ownerPane, targetRef); err != nil {
					return err
				}
			}
			if !ok {
				return fmt.Errorf("owner-scoped sidebar switch requires atomic tmux move+switch support")
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
			saveSidebarVisibilityAfterCommittedSwitch(ctx, client)
			return nil
		}
	}
	output, err := tmux(ctx, appendSwitchClientArgs(client, sessionName)...)
	if err != nil {
		return tmuxCommandError("switch client session", output, err)
	}
	return nil
}

func bestTargetSidebarPane(ctx context.Context, sidebar ports.SidebarPort, target string, client string, ownerPane ports.PaneRef) (ports.PaneRef, error) {
	if finder, ok := sidebar.(ports.SidebarTargetPanesPort); ok {
		refs, err := finder.FindSidebarPanes(ctx, target)
		if err != nil {
			return ports.PaneRef{}, err
		}
		return selectBestTargetSidebarPane(ctx, refs, client, ownerPane)
	}
	return sidebar.FindSidebarPane(ctx, target)
}

func selectBestTargetSidebarPane(ctx context.Context, refs []ports.PaneRef, client string, ownerPane ports.PaneRef) (ports.PaneRef, error) {
	if len(refs) == 0 {
		return ports.PaneRef{}, nil
	}
	if ownerPane.PaneID != "" {
		for _, ref := range refs {
			if ref.PaneID == ownerPane.PaneID {
				return ref, nil
			}
		}
	}
	client = strings.TrimSpace(client)
	for _, ref := range refs {
		if strings.TrimSpace(ref.OwnerClientID) == client {
			return ref, nil
		}
	}
	for _, ref := range refs {
		owner := strings.TrimSpace(ref.OwnerClientID)
		if owner == "" || owner == client {
			continue
		}
		reusable, err := targetSidebarReusableForClient(ctx, ref, client)
		if err != nil {
			return ports.PaneRef{}, err
		}
		if reusable {
			return ref, nil
		}
	}
	for _, ref := range refs {
		if strings.TrimSpace(ref.OwnerClientID) == "" {
			return ref, nil
		}
	}
	return refs[0], nil
}

func adoptTargetSidebar(ctx context.Context, mover ports.SidebarSwitchPort, client string, sessionName string, targetRef ports.PaneRef, cfg ports.ConfigSnapshot) error {
	if err := mover.AttachSidebarForClientAndSwitchClient(ctx, client, sessionName, targetRef.PaneID, cfg.Width); err != nil {
		return fmt.Errorf("adopt target sidebar and switch to %q: %w", sessionName, err)
	}
	return nil
}

func parkPreviousSidebarAfterAdoption(ctx context.Context, sidebar ports.SidebarPort, client string, ownerPane ports.PaneRef, targetRef ports.PaneRef) {
	if ownerPane.PaneID == "" || ownerPane.PaneID == targetRef.PaneID {
		return
	}
	if err := sidebar.ParkSidebarForClient(ctx, client, ownerPane.PaneID); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: park previous sidebar after switch failed for client %q pane %q: %v\n", client, ownerPane.PaneID, err)
	}
}

func parkNonReusableTargetSidebar(ctx context.Context, sidebar ports.SidebarPort, client string, ownerPane ports.PaneRef, targetRef ports.PaneRef) error {
	if targetRef.PaneID == "" || targetRef.PaneID == ownerPane.PaneID {
		return nil
	}
	targetOwner := strings.TrimSpace(targetRef.OwnerClientID)
	if targetOwner == "" {
		targetOwner = client
	}
	return sidebar.ParkSidebarForClient(ctx, targetOwner, targetRef.PaneID)
}

func targetSidebarReusableForClient(ctx context.Context, targetRef ports.PaneRef, client string) (bool, error) {
	owner := strings.TrimSpace(targetRef.OwnerClientID)
	if owner == "" || owner == strings.TrimSpace(client) {
		return true, nil
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		return false, err
	}
	return sidebarStateAppliesToClient(state, owner), nil
}

func saveSidebarVisibilityAfterCommittedSwitch(ctx context.Context, client string) {
	if err := saveSidebarVisibility(ctx, true, client); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-session-sidebar: save sidebar visibility after switch failed: %v\n", err)
	}
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
