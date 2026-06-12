package app

import (
	"context"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSidebarOwnerResolverUsesExplicitClient(t *testing.T) {
	resolver := sidebarOwnerResolver{environ: func(string) string { return "%sidebar" }}
	if got := resolver.ResolveActionClient(context.Background(), map[string]string{"client": " client-2 "}); got != "client-2" {
		t.Fatalf("ResolveActionClient explicit = %q, want client-2", got)
	}
}

func TestSidebarOwnerResolverUsesUniqueClientViewingSidebarPaneBeforePersistedOwner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1 $2" in
  "list-clients -F")
    case "$3" in
      "#{client_name}") printf 'client-owner\nclient-viewer\n' ;;
      *) printf 'client-owner\t@2\talpha\nclient-viewer\t@1\tbeta\nclient-sidebar\t@1\ttmux-session-sidebar\n' ;;
    esac ;;
  "display-message -p") printf '@1\n' ;;
esac
`)
	ctx := context.Background()
	if err := saveSidebarVisibility(ctx, true, "client-owner"); err != nil {
		t.Fatalf("saveSidebarVisibility: %v", err)
	}

	resolver := sidebarOwnerResolver{environ: func(name string) string {
		if name == "TMUX_PANE" {
			return "%sidebar"
		}
		return ""
	}}
	if got := resolver.ResolveActionClient(ctx, nil); got != "client-viewer" {
		t.Fatalf("ResolveActionClient = %q, want unique sidebar viewer", got)
	}
}

func TestSidebarOwnerResolverFallsBackToPersistedOwnerWhenSidebarHidden(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: false, OwnerClient: "client-owner"}
	}); err != nil {
		t.Fatalf("seed sidebar state: %v", err)
	}

	resolver := sidebarOwnerResolver{environ: func(string) string { return "" }}
	if got := resolver.ResolveActionClient(ctx, nil); got != "client-owner" {
		t.Fatalf("ResolveActionClient hidden sidebar = %q, want persisted owner", got)
	}
}

func TestSidebarOwnerResolverFallsBackToClientViewingSidebarPaneWhenOwnerStale(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1 $2" in
  "list-clients -F")
    case "$3" in
      "#{client_name}") printf 'client-2\n' ;;
      *) printf 'client-2\t@2\talpha\nclient-sidebar\t@1\ttmux-session-sidebar\n' ;;
    esac ;;
  "display-message -p") printf '@2\n' ;;
esac
`)
	ctx := context.Background()
	if err := saveSidebarVisibility(ctx, true, "client-1"); err != nil {
		t.Fatalf("saveSidebarVisibility: %v", err)
	}

	resolver := sidebarOwnerResolver{environ: func(name string) string {
		if name == "TMUX_PANE" {
			return "%sidebar"
		}
		return ""
	}}
	if got := resolver.ResolveActionClient(ctx, nil); got != "client-2" {
		t.Fatalf("ResolveActionClient stale owner = %q, want client-2", got)
	}
}

func TestSidebarOwnerResolverFallsBackToStaleOwnerWhenNoViewingClientInferred(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1 $2" in
  "list-clients -F") printf 'client-2\n' ;;
esac
`)
	ctx := context.Background()
	if err := saveSidebarVisibility(ctx, true, "client-1"); err != nil {
		t.Fatalf("saveSidebarVisibility: %v", err)
	}

	resolver := sidebarOwnerResolver{environ: func(name string) string {
		if name == "TMUX_PANE" {
			return ""
		}
		return ""
	}}
	if got := resolver.ResolveActionClient(ctx, nil); got != "client-1" {
		t.Fatalf("ResolveActionClient stale owner without viewing client = %q, want client-1", got)
	}
}

func TestSidebarOwnerResolverFallsBackToPersistedOwnerWhenSidebarViewerAmbiguous(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1 $2" in
  "list-clients -F")
    case "$3" in
      "#{client_name}") printf 'client-a\nclient-b\n' ;;
      *) printf 'client-a\t@1\talpha\nclient-b\t@1\tbeta\nclient-sidebar\t@1\ttmux-session-sidebar\n' ;;
    esac ;;
  "display-message -p") printf '@1\n' ;;
esac
`)
	ctx := context.Background()
	if err := saveSidebarVisibility(ctx, true, "client-owner"); err != nil {
		t.Fatalf("saveSidebarVisibility: %v", err)
	}

	resolver := sidebarOwnerResolver{environ: func(name string) string {
		if name == "TMUX_PANE" {
			return "%sidebar"
		}
		return ""
	}}
	if got := resolver.ResolveActionClient(ctx, nil); got != "client-owner" {
		t.Fatalf("ResolveActionClient ambiguous viewers = %q, want persisted owner", got)
	}
}

func TestSidebarOwnerResolverDoesNotFallbackToInternalClient(t *testing.T) {
	installFakeTmux(t, `#!/usr/bin/env bash
case "$1" in
  display-message) printf '@1\n' ;;
  list-clients) printf '/dev/sidebar\t@1\ttmux-session-sidebar\n/dev/work\t@2\talpha\n' ;;
esac
`)
	resolver := sidebarOwnerResolver{environ: func(name string) string {
		if name == "TMUX_PANE" {
			return "%sidebar"
		}
		return ""
	}}

	if got := resolver.uniqueClientViewingVisibleSidebarPane(context.Background()); got != "" {
		t.Fatalf("uniqueClientViewingVisibleSidebarPane = %q, want empty when only internal client views sidebar pane", got)
	}
}

func TestSidebarOwnerResolverAdoptsOnlyOpenSidebar(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ctx := context.Background()
	resolver := sidebarOwnerResolver{environ: func(string) string { return "" }}

	if err := resolver.AdoptOpenSidebar(ctx, "client-1"); err != nil {
		t.Fatalf("AdoptOpenSidebar closed: %v", err)
	}
	state, err := persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState closed: %v", err)
	}
	if state.OwnerClient != "" {
		t.Fatalf("closed owner = %q, want empty", state.OwnerClient)
	}

	if err := updateSidebarState(ctx, func(state *ports.PersistedState) {
		state.Sidebar = &ports.SidebarState{Open: true, OwnerClient: "client-old"}
	}); err != nil {
		t.Fatalf("seed open sidebar: %v", err)
	}
	if err := resolver.AdoptOpenSidebar(ctx, "client-new"); err != nil {
		t.Fatalf("AdoptOpenSidebar open: %v", err)
	}
	state, err = persistedSidebarState(ctx)
	if err != nil {
		t.Fatalf("persistedSidebarState open: %v", err)
	}
	if state.OwnerClient != "client-new" {
		t.Fatalf("open owner = %q, want client-new", state.OwnerClient)
	}
}
