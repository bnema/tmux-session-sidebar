package app

import (
	"context"
	"os"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type sidebarOwnerResolver struct {
	environ func(string) string
}

func newSidebarOwnerResolver() sidebarOwnerResolver {
	return sidebarOwnerResolver{environ: os.Getenv}
}

func (r sidebarOwnerResolver) ResolveActionClient(ctx context.Context, flags map[string]string) string {
	if client := strings.TrimSpace(flags["client"]); client != "" {
		return client
	}
	state, err := persistedSidebarState(ctx)
	if err != nil || !state.Open {
		return ""
	}
	owner := strings.TrimSpace(state.OwnerClient)
	if owner == "" {
		return r.clientViewingSidebarPane(ctx)
	}
	if tmuxClientExists(ctx, owner) {
		return owner
	}
	if viewingClient := r.clientViewingSidebarPane(ctx); viewingClient != "" {
		return viewingClient
	}
	return owner
}

func (r sidebarOwnerResolver) AdoptOpenSidebar(ctx context.Context, client string) error {
	client = strings.TrimSpace(client)
	if client == "" {
		return nil
	}
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		if state.Sidebar == nil || !state.Sidebar.Open {
			return
		}
		state.Sidebar.OwnerClient = client
	})
}

func tmuxClientExists(ctx context.Context, client string) bool {
	out, err := tmux(ctx, "list-clients", "-F", "#{client_name}")
	if err != nil {
		return true
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == client {
			return true
		}
	}
	return false
}

func (r sidebarOwnerResolver) clientViewingSidebarPane(ctx context.Context) string {
	pane := strings.TrimSpace(r.environ("TMUX_PANE"))
	if pane == "" {
		return ""
	}
	windowID, err := tmux(ctx, "display-message", "-p", "-t", pane, "#{window_id}")
	if err != nil {
		return ""
	}
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	out, err := tmux(ctx, "list-clients", "-F", "#{client_name}\t#{window_id}\t#{client_session}")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 || isInternalHookSession(fields[2]) {
			continue
		}
		if strings.TrimSpace(fields[1]) == windowID {
			return strings.TrimSpace(fields[0])
		}
	}
	return ""
}
