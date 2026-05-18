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

	"github.com/bnema/tmux-session-sidebar/adapters/process"
	"github.com/bnema/tmux-session-sidebar/adapters/tmuxcli"
	"github.com/bnema/tmux-session-sidebar/core/projects"
	coreruntime "github.com/bnema/tmux-session-sidebar/core/runtime"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func createProject(ctx context.Context, flags map[string]string, stdout io.Writer) error {
	projectPath := flags["project-path"]
	if projectPath == "" {
		selected, err := pickProject(ctx, stdout)
		if err != nil || selected == "" {
			return err
		}
		projectPath = selected
	}
	candidate := projects.CandidateFromPath(projectPath)
	return createOrSwitchProject(ctx, flags["client"], candidate)
}

func projectCandidates(ctx context.Context) ([]projects.Candidate, error) {
	rootsOut, err := tmux(ctx, "show-options", "-gvq", "@session-sidebar-project-roots")
	if err != nil {
		return nil, err
	}
	var candidates []projects.Candidate
	for root := range strings.SplitSeq(strings.TrimSpace(rootsOut), ":") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root = os.ExpandEnv(root)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
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

func createOrSwitchProject(ctx context.Context, client string, candidate projects.Candidate) error {
	existing, err := loadSessionViews(ctx)
	if err != nil {
		return err
	}
	return withSidebarFollow(ctx, client, func() error {
		return runtimeService().CreateProjectSession(ctx, client, existing, candidate)
	})
}

func switchClient(ctx context.Context, client string, sessionName string) error {
	return withSidebarFollow(ctx, client, func() error {
		return runtimeService().SwitchSession(ctx, client, sessionName)
	})
}

func loadSessionViews(ctx context.Context) ([]sessions.View, error) {
	return runtimeService().SessionViews(ctx)
}

func runtimeService() *coreruntime.Service {
	tmux := tmuxcli.Client{Process: process.Runner{}}
	return coreruntime.NewService(nil, tmux, tmux, nil).WithMetadata(tmux)
}
