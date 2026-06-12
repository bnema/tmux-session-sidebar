package tmuxcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	processadapter "github.com/bnema/tmux-session-sidebar/adapters/process"
)

func TestAttachSingletonSidebarReopensBesideBottomFocusedStackedPanes(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}

	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_width}\t#{pane_height}")
	assertSidebarIsFullHeightLeftColumn(t, rows, sidebarPane)
}

func TestAttachSingletonSidebarPreservesVisibleStackedPaneProportions(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "resize-pane", "-D", "5")
	want := paneGeometryByID(t, runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}"))

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := paneGeometryByID(t, runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible stacked pane geometry changed after hide/reopen\nwant: %#v\n got: %#v", want, got)
	}
}

func TestAttachSingletonSidebarPreservesVisibleRightSplit(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "work:")
	want := paneGeometryByID(t, runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}"))

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := paneGeometryByID(t, runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible right-split geometry changed after hide/reopen\nwant: %#v\n got: %#v", want, got)
	}
}

// ─── Priority scenario 1: Visible-side edits then toggle ─────────────────

// TestAttachSingletonSidebarPreservesRightSplitWithManualResize verifies that
// a right split combined with manual pane resizing survives park+re-attach.
// This extends TestAttachSingletonSidebarPreservesVisibleRightSplit with an
// explicit resize step to characterize percentage-change memory.
func TestAttachSingletonSidebarPreservesRightSplitWithManualResize(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	// Create a right split (horizontal division) in the work area.
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "work:")
	// Manually resize one of the right-side panes downward to alter the split ratio.
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "resize-pane", "-D", "3")
	want := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertGeometriesUnchanged(t, want, got)
}

// TestAttachSingletonSidebarPreservesWorkPaneManualResizeWithoutSplit verifies
// that resizing a work pane (changing its proportion) survives park+re-attach
// when no structural split changes occur.
func TestAttachSingletonSidebarPreservesWorkPaneManualResizeWithoutSplit(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	// Resize the bottom work pane upward to alter the vertical split ratio.
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "resize-pane", "-U", "4")
	want := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertGeometriesUnchanged(t, want, got)
}

// ─── Priority scenario 2: Hidden-side edits then reopen ─────────────────

// TestAttachSingletonSidebarPreservesHiddenSideEditsInWorkArea verifies that the
// live tmux layout is authoritative: if the user modifies the work-window layout
// while the sidebar is parked (hidden-side edits), re-attaching the sidebar
// preserves those hidden-side edits in the work area's geometry rather than
// overwriting them with a stale visible-layout snapshot.
func TestAttachSingletonSidebarPreservesHiddenSideEditsInWorkArea(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	visibleLayout := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	parkedLayout := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))

	// Reorganize the work-window layout while sidebar is hidden (hidden-side edit).
	runTmux(t, ctx, realTmux, socketName, "select-layout", "-t", "work:", "main-horizontal")
	hiddenEditLayout := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))
	if hiddenEditLayout == parkedLayout {
		t.Fatalf("hidden-side edit did not change the parked work-window layout\nparked: %s\n edited: %s", parkedLayout, hiddenEditLayout)
	}

	// Record work-pane geometries after the hidden-side edit (before re-attach).
	wantWorkGeometry := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	reAttachedLayout := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))

	// The re-attached layout must NOT be the stale visible layout — the live
	// tmux layout (including hidden-side edits) is authoritative.
	if reAttachedLayout == visibleLayout {
		t.Fatalf("reattached layout reused the stale visible layout; expected hidden-side edit to be preserved\nvisible:    %s\nparkedEdit: %s\nreattached: %s", visibleLayout, hiddenEditLayout, reAttachedLayout)
	}

	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	geo, ok := got[sidebarPane]
	if !ok {
		t.Fatalf("sidebar pane %s not found after re-attach", sidebarPane)
	}
	assertSidebarGeometryHasPositiveSize(t, geo)

	// Verify the work-area panes from the hidden-side edit are still present
	// with unchanged geometry (sidebar insert shrinks width but preserves
	// top/height and proportion).
	for paneID, wantGeo := range wantWorkGeometry {
		if paneID == sidebarPane {
			continue
		}
		gotGeo, ok := got[paneID]
		if !ok {
			t.Errorf("work pane %s missing after re-attach (was present in hidden-side edit)", paneID)
			continue
		}
		// Only top and height must be preserved; left shifts rightward and
		// width shrinks because the sidebar column occupies the left edge.
		wantParts := strings.Split(wantGeo, ",")
		gotParts := strings.Split(gotGeo, ",")
		if len(wantParts) != 4 || len(gotParts) != 4 {
			t.Errorf("pane %s malformed geometry: want=%q got=%q", paneID, wantGeo, gotGeo)
			continue
		}
		if gotParts[1] != wantParts[1] {
			t.Errorf("pane %s top changed: want=%s got=%s", paneID, wantParts[1], gotParts[1])
		}
		if gotParts[3] != wantParts[3] {
			t.Errorf("pane %s height changed: want=%s got=%s", paneID, wantParts[3], gotParts[3])
		}
	}
}

// ─── Priority scenario 3: Repeated toggle cycles ────────────────────────

// TestAttachSingletonSidebarRepeatedToggleCycles performs multiple park+attach
// cycles and verifies that pane geometry does not degrade over repeated toggles.
func TestAttachSingletonSidebarRepeatedToggleCycles(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	client, sidebarPane := setupStackedWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("baseline AttachSingletonSidebar error: %v", err)
	}
	want := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("baseline ParkSingletonSidebar error: %v", err)
	}

	for i := range 3 {
		if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
			t.Fatalf("AttachSingletonSidebar cycle %d error: %v", i, err)
		}
		if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
			t.Fatalf("ParkSingletonSidebar cycle %d error: %v", i, err)
		}
	}

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("final AttachSingletonSidebar error: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertGeometriesUnchanged(t, want, got)
}

// ─── Priority scenario 4: Complex geometry & edge cases ─────────────────

// TestAttachSingletonSidebarNestedSplitPreserved verifies that a nested split
// (a pane that is split both vertically and horizontally) is preserved through
// a park+re-attach cycle.
func TestAttachSingletonSidebarNestedSplitPreserved(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	// Create a work session with nested splits: a vertical split,
	// then one side split further horizontally.
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "100", "-y", "30")
	runTmux(t, ctx, realTmux, socketName, "set-option", "-g", "pane-base-index", "1")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "work:")

	// Hidden sidebar session.
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "20", "-y", "30")
	sidebarPane := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", singletonSidebarSessionName+":", "#{pane_id}"))
	runTmux(t, ctx, realTmux, socketName, "set-option", "-p", "-t", sidebarPane, optionSidebarPane, "1")
	client := Client{Process: processadapter.Runner{}}

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
	want := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertGeometriesUnchanged(t, want, got)
}

// TestAttachSingletonSidebarMovePreservesVisibleEdits verifies that moving the
// singleton sidebar to another session does not replay a stale pre-sidebar
// source layout. Visible-side edits made in the source work area must survive
// when the sidebar leaves that session.
func TestAttachSingletonSidebarMovePreservesVisibleEdits(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "100", "-y", "30")
	runTmux(t, ctx, realTmux, socketName, "set-option", "-g", "pane-base-index", "1")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	workHidden := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "other", "-x", "100", "-y", "30")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "other:")

	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "20", "-y", "30")
	sidebarPane := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", singletonSidebarSessionName+":", "#{pane_id}"))
	runTmux(t, ctx, realTmux, socketName, "set-option", "-p", "-t", sidebarPane, optionSidebarPane, "1")
	client := Client{Process: processadapter.Runner{}}

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("attach to work error: %v", err)
	}
	// Visible-side edit: add an extra work pane while the sidebar is attached.
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", sidebarPane, "-R")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "work:")
	visibleEditedWork := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	if len(visibleEditedWork) <= len(workHidden) {
		t.Fatalf("visible-side edit did not add a work pane: hidden=%#v visible=%#v", workHidden, visibleEditedWork)
	}

	if _, err := client.AttachSingletonSidebar(ctx, "other:", sidebarPane, "20"); err != nil {
		t.Fatalf("move attach to other error: %v", err)
	}

	gotWork := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	if len(gotWork) != len(visibleEditedWork)-1 {
		t.Fatalf("source work session lost visible-side edits after move: want %d work panes, got %d\nhidden=%#v\nvisible=%#v\ngot=%#v", len(visibleEditedWork)-1, len(gotWork), workHidden, visibleEditedWork, gotWork)
	}
	for paneID := range visibleEditedWork {
		if paneID == sidebarPane {
			continue
		}
		if _, ok := gotWork[paneID]; !ok {
			t.Fatalf("source work session lost pane %s after move; visible edit was not preserved\nvisible=%#v\ngot=%#v", paneID, visibleEditedWork, gotWork)
		}
	}
	gotOther := recordPaneGeometries(t, ctx, realTmux, socketName, "other:")
	geo, ok := gotOther[sidebarPane]
	if !ok {
		t.Fatalf("sidebar pane %s not found in target session after move", sidebarPane)
	}
	assertSidebarGeometryHasPositiveSize(t, geo)
}

// TestAttachSingletonSidebarSmallWindowNoCrash verifies that attaching the
// sidebar to a very small window does not cause tmux errors or produce
// degenerate geometry.
func TestAttachSingletonSidebarSmallWindowNoCrash(t *testing.T) {
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	ctx := t.Context()
	socketName := tmuxTestSocketName(t)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), realTmux, "-f", "/dev/null", "-L", socketName, "kill-server").Run()
	})
	installTmuxSocketWrapper(t, realTmux, socketName)

	// Small window: 50 columns x 10 rows.
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "50", "-y", "10")
	runTmux(t, ctx, realTmux, socketName, "set-option", "-g", "pane-base-index", "1")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")

	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "20", "-y", "10")
	sidebarPane := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", singletonSidebarSessionName+":", "#{pane_id}"))
	runTmux(t, ctx, realTmux, socketName, "set-option", "-p", "-t", sidebarPane, optionSidebarPane, "1")
	client := Client{Process: processadapter.Runner{}}

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("AttachSingletonSidebar error on small window: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 panes, got %d: %#v", len(got), got)
	}
	geo, ok := got[sidebarPane]
	if !ok {
		t.Fatalf("sidebar pane %s not found in small window", sidebarPane)
	}
	assertSidebarGeometryHasPositiveSize(t, geo)

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error on small window: %v", err)
	}
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error on small window: %v", err)
	}
	got = recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	geo, ok = got[sidebarPane]
	if !ok {
		t.Fatalf("sidebar pane %s not found after small-window re-attach", sidebarPane)
	}
	assertSidebarGeometryHasPositiveSize(t, geo)
}

func setupStackedWorkSessionWithSidebar(t *testing.T, ctx context.Context, realTmux string, socketName string) (Client, string) {
	t.Helper()
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "100", "-y", "30")
	runTmux(t, ctx, realTmux, socketName, "set-option", "-g", "pane-base-index", "1")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "select-pane", "-t", "work:", "-D")
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "20", "-y", "30")
	sidebarPane := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", singletonSidebarSessionName+":", "#{pane_id}"))
	runTmux(t, ctx, realTmux, socketName, "set-option", "-p", "-t", sidebarPane, optionSidebarPane, "1")
	return Client{Process: processadapter.Runner{}}, sidebarPane
}

func tmuxTestSocketName(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-", "_", "-").Replace(t.Name())
	return fmt.Sprintf("tss-test-%d-%s", os.Getpid(), name)
}

func installTmuxSocketWrapper(t *testing.T, realTmux string, socketName string) {
	t.Helper()
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "tmux")
	content := fmt.Sprintf("#!/usr/bin/env bash\nexec %q -f /dev/null -L %q \"$@\"\n", realTmux, socketName)
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		t.Fatalf("write tmux wrapper: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runTmux(t *testing.T, ctx context.Context, realTmux string, socketName string, args ...string) {
	t.Helper()
	_ = runTmuxOutput(t, ctx, realTmux, socketName, args...)
}

func runTmuxOutput(t *testing.T, ctx context.Context, realTmux string, socketName string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-f", "/dev/null", "-L", socketName}, args...)
	output, err := exec.CommandContext(ctx, realTmux, cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

func paneGeometryByID(t *testing.T, rows string) map[string]string {
	t.Helper()
	geometry := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			t.Fatalf("malformed list-panes row %q", line)
		}
		geometry[fields[0]] = strings.Join(fields[1:], ",")
	}
	return geometry
}

func assertSidebarIsFullHeightLeftColumn(t *testing.T, rows string, sidebarPane string) {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			t.Fatalf("malformed list-panes row %q", line)
		}
		if fields[0] == sidebarPane {
			if fields[1] != "0" || fields[2] != "20" || fields[3] != "30" {
				t.Fatalf("sidebar pane geometry = left %s width %s height %s, want full-height left column 0/20/30\nall panes:\n%s", fields[1], fields[2], fields[3], rows)
			}
			return
		}
	}
	t.Fatalf("sidebar pane %s not found in panes:\n%s", sidebarPane, rows)
}

// recordPaneGeometries returns a map of pane_id -> "left,top,width,height" for all panes in a session.
func recordPaneGeometries(t *testing.T, ctx context.Context, realTmux string, socketName string, session string) map[string]string {
	t.Helper()
	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", session, "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}")
	return paneGeometryByID(t, rows)
}

// assertGeometriesUnchanged fails if any pane geometry in got differs from want.
func assertGeometriesUnchanged(t *testing.T, want map[string]string, got map[string]string) {
	t.Helper()
	for id, wantGeo := range want {
		gotGeo, ok := got[id]
		if !ok {
			t.Errorf("pane %s missing from re-attached layout", id)
			continue
		}
		if gotGeo != wantGeo {
			t.Errorf("pane %s geometry changed\n  want: %s\n  got:  %s", id, wantGeo, gotGeo)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected pane %s appeared in re-attached layout\n  geo: %s", id, got[id])
		}
	}
}

func assertSidebarGeometryHasPositiveSize(t *testing.T, geometry string) {
	t.Helper()
	fields := strings.Split(geometry, ",")
	if len(fields) != 4 {
		t.Fatalf("malformed sidebar geometry %q, want left,top,width,height", geometry)
	}
	left, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("parse sidebar left from %q: %v", geometry, err)
	}
	width, err := strconv.Atoi(fields[2])
	if err != nil {
		t.Fatalf("parse sidebar width from %q: %v", geometry, err)
	}
	height, err := strconv.Atoi(fields[3])
	if err != nil {
		t.Fatalf("parse sidebar height from %q: %v", geometry, err)
	}
	if left != 0 {
		t.Fatalf("sidebar left = %d, want 0 for left column geometry %q", left, geometry)
	}
	if width <= 0 {
		t.Fatalf("sidebar width = %d, want positive geometry %q", width, geometry)
	}
	if height <= 0 {
		t.Fatalf("sidebar height = %d, want positive geometry %q", height, geometry)
	}
}
