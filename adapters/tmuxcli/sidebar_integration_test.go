package tmuxcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	processadapter "github.com/bnema/tmux-session-sidebar/adapters/process"
)

func TestAttachSingletonSidebarPreservesGroupedHorizontalWorkSplitAcrossToggle(t *testing.T) {
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

	client, sidebarPane := setupFlatWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "30"); err != nil {
		t.Fatalf("first AttachSingletonSidebar error: %v", err)
	}
	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{?@session-sidebar-pane,1,0}")
	leftWorkPane := leftmostWorkPaneID(t, rows)
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", leftWorkPane)
	wantOpen := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	if err := client.ParkSingletonSidebar(ctx, sidebarPane); err != nil {
		t.Fatalf("ParkSingletonSidebar error: %v", err)
	}
	assertPaneWidths(t, recordPaneGeometries(t, ctx, realTmux, socketName, "work:"), []int{90, 90})

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "30"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertGeometriesUnchanged(t, wantOpen, got)
}

func TestSyncAttachedSidebarWidthPreservesGroupedWorkRatiosAcrossWindowResize(t *testing.T) {
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

	client, sidebarPane := setupFlatWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "30"); err != nil {
		t.Fatalf("AttachSingletonSidebar error: %v", err)
	}
	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", "work:", "-F", "#{pane_id}\t#{pane_left}\t#{?@session-sidebar-pane,1,0}")
	rightWorkPane := rightmostWorkPaneID(t, rows)
	runTmux(t, ctx, realTmux, socketName, "split-window", "-v", "-t", rightWorkPane)
	wantOpen := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertPaneWidths(t, geometriesWithoutPane(wantOpen, sidebarPane), []int{74, 75})
	windowID := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_id}"))

	type attachedSidebarWidthSyncer interface {
		CaptureAttachedSidebarWidthBaseline(context.Context, string, string, string) error
		SyncAttachedSidebarWidth(context.Context, string, string, string) error
	}
	syncer, ok := any(client).(attachedSidebarWidthSyncer)
	if !ok {
		t.Fatalf("Client does not implement sidebar width sync")
	}
	if err := syncer.CaptureAttachedSidebarWidthBaseline(ctx, windowID, sidebarPane, "30"); err != nil {
		t.Fatalf("CaptureAttachedSidebarWidthBaseline error: %v", err)
	}

	runTmux(t, ctx, realTmux, socketName, "resize-window", "-t", "work:", "-x", "59", "-y", "48")
	if err := syncer.SyncAttachedSidebarWidth(ctx, windowID, sidebarPane, "30"); err != nil {
		t.Fatalf("SyncAttachedSidebarWidth after narrow resize error: %v", err)
	}

	runTmux(t, ctx, realTmux, socketName, "resize-window", "-t", "work:", "-x", "181", "-y", "48")
	if err := syncer.SyncAttachedSidebarWidth(ctx, windowID, sidebarPane, "30"); err != nil {
		t.Fatalf("SyncAttachedSidebarWidth after restore error: %v", err)
	}

	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertPaneWidths(t, geometriesWithoutPane(got, sidebarPane), []int{74, 75})
	for paneID, wantGeo := range wantOpen {
		gotGeo, ok := got[paneID]
		if !ok {
			t.Fatalf("pane %s missing after resize restore", paneID)
		}
		wantParts := strings.Split(wantGeo, ",")
		gotParts := strings.Split(gotGeo, ",")
		if len(wantParts) != 4 || len(gotParts) != 4 {
			t.Fatalf("malformed geometry after resize restore: want=%q got=%q", wantGeo, gotGeo)
		}
		if gotParts[1] != wantParts[1] {
			t.Errorf("pane %s top changed after resize restore: want=%s got=%s", paneID, wantParts[1], gotParts[1])
		}
		if gotParts[3] != wantParts[3] {
			t.Errorf("pane %s height changed after resize restore: want=%s got=%s", paneID, wantParts[3], gotParts[3])
		}
	}
}

func TestAttachSidebarSessionRoundTripKeepsWorkSplit(t *testing.T) {
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

	client, sidebarPane := setupFlatWorkSessionWithSidebar(t, ctx, realTmux, socketName)
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "other", "-x", "181", "-y", "48")

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "30"); err != nil {
		t.Fatalf("AttachSingletonSidebar work error: %v", err)
	}
	wantOpen := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	assertPaneWidths(t, geometriesWithoutPane(wantOpen, sidebarPane), []int{74, 75})

	if _, err := client.AttachSingletonSidebarWithoutFocus(ctx, "other:", sidebarPane, "30"); err != nil {
		t.Fatalf("AttachSingletonSidebarWithoutFocus other error: %v", err)
	}
	assertPaneWidths(t, recordPaneGeometries(t, ctx, realTmux, socketName, "work:"), []int{90, 90})

	if _, err := client.AttachSingletonSidebarWithoutFocus(ctx, "work:", sidebarPane, "30"); err != nil {
		t.Fatalf("AttachSingletonSidebarWithoutFocus work error: %v", err)
	}
	assertGeometriesUnchanged(t, wantOpen, recordPaneGeometries(t, ctx, realTmux, socketName, "work:"))
}

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
	parkedGeometry := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")

	hiddenEditLayout := parkedLayout
	wantWorkGeometry := parkedGeometry
	for _, preset := range []string{"main-horizontal", "tiled", "main-vertical"} {
		runTmux(t, ctx, realTmux, socketName, "select-layout", "-t", "work:", preset)
		hiddenEditLayout = strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))
		wantWorkGeometry = recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
		if !reflect.DeepEqual(wantWorkGeometry, parkedGeometry) {
			break
		}
	}
	if reflect.DeepEqual(wantWorkGeometry, parkedGeometry) {
		t.Fatalf("hidden-side edit did not change the parked work-window geometry\nparked layout: %s\n edited layout: %s\nparked geometry: %#v\n edited geometry: %#v", parkedLayout, hiddenEditLayout, parkedGeometry, wantWorkGeometry)
	}

	if _, err := client.AttachSingletonSidebar(ctx, "work:", sidebarPane, "20"); err != nil {
		t.Fatalf("second AttachSingletonSidebar error: %v", err)
	}
	reAttachedLayout := strings.TrimSpace(runTmuxOutput(t, ctx, realTmux, socketName, "display-message", "-p", "-t", "work:", "#{window_layout}"))
	if reAttachedLayout == visibleLayout {
		t.Fatalf("reattached layout reused the stale visible layout; expected hidden-side edit to be preserved\nvisible:    %s\nparkedEdit: %s\nreattached: %s", visibleLayout, hiddenEditLayout, reAttachedLayout)
	}

	got := recordPaneGeometries(t, ctx, realTmux, socketName, "work:")
	geo, ok := got[sidebarPane]
	if !ok {
		t.Fatalf("sidebar pane %s not found after re-attach", sidebarPane)
	}
	assertSidebarGeometryHasPositiveSize(t, geo)

	for paneID, wantGeo := range wantWorkGeometry {
		if paneID == sidebarPane {
			continue
		}
		gotGeo, ok := got[paneID]
		if !ok {
			t.Errorf("work pane %s missing after re-attach (was present in hidden-side edit)", paneID)
			continue
		}
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

func setupFlatWorkSessionWithSidebar(t *testing.T, ctx context.Context, realTmux string, socketName string) (Client, string) {
	t.Helper()
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", "work", "-x", "181", "-y", "48")
	runTmux(t, ctx, realTmux, socketName, "set-option", "-g", "pane-base-index", "1")
	runTmux(t, ctx, realTmux, socketName, "split-window", "-h", "-t", "work:")
	runTmux(t, ctx, realTmux, socketName, "new-session", "-d", "-s", singletonSidebarSessionName, "-n", singletonSidebarWindowName, "-x", "30", "-y", "48")
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

func recordPaneGeometries(t *testing.T, ctx context.Context, realTmux string, socketName string, session string) map[string]string {
	t.Helper()
	rows := runTmuxOutput(t, ctx, realTmux, socketName, "list-panes", "-t", session, "-F", "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}")
	return paneGeometryByID(t, rows)
}

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

func geometriesWithoutPane(geometries map[string]string, paneID string) map[string]string {
	filtered := make(map[string]string, len(geometries))
	for id, geometry := range geometries {
		if id == paneID {
			continue
		}
		filtered[id] = geometry
	}
	return filtered
}

func assertPaneWidths(t *testing.T, geometries map[string]string, want []int) {
	t.Helper()
	got := append([]int(nil), topLevelGroupWidthsFromGeometries(t, geometries)...)
	want = append([]int(nil), want...)
	sort.Ints(got)
	sort.Ints(want)
	if len(got) != len(want) {
		t.Fatalf("hidden work widths = %#v, got widths %v want %v", geometries, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hidden work widths = %#v, got widths %v want %v", geometries, got, want)
		}
	}
}

func topLevelGroupWidthsFromGeometries(t *testing.T, geometries map[string]string) []int {
	t.Helper()
	byLeft := map[int]int{}
	for _, geometry := range geometries {
		fields := strings.Split(geometry, ",")
		if len(fields) != 4 {
			t.Fatalf("malformed pane geometry %q", geometry)
		}
		left, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatalf("parse pane left from %q: %v", geometry, err)
		}
		width, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("parse pane width from %q: %v", geometry, err)
		}
		if width > byLeft[left] {
			byLeft[left] = width
		}
	}
	lefts := make([]int, 0, len(byLeft))
	for left := range byLeft {
		lefts = append(lefts, left)
	}
	sort.Ints(lefts)
	widths := make([]int, 0, len(lefts))
	for _, left := range lefts {
		widths = append(widths, byLeft[left])
	}
	return widths
}

func leftmostWorkPaneID(t *testing.T, rows string) string {
	t.Helper()
	bestID := ""
	bestLeft := 0
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("malformed work-pane row %q", line)
		}
		if fields[2] == "1" {
			continue
		}
		left, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("parse pane left from %q: %v", line, err)
		}
		if bestID == "" || left < bestLeft {
			bestID = fields[0]
			bestLeft = left
		}
	}
	if bestID == "" {
		t.Fatalf("no non-sidebar work pane found in rows:\n%s", rows)
	}
	return bestID
}

func rightmostWorkPaneID(t *testing.T, rows string) string {
	t.Helper()
	bestID := ""
	bestLeft := 0
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("malformed work-pane row %q", line)
		}
		if fields[2] == "1" {
			continue
		}
		left, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("parse pane left from %q: %v", line, err)
		}
		if bestID == "" || left > bestLeft {
			bestID = fields[0]
			bestLeft = left
		}
	}
	if bestID == "" {
		t.Fatalf("no non-sidebar work pane found in rows:\n%s", rows)
	}
	return bestID
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
	top, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("parse sidebar top from %q: %v", geometry, err)
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
	if top != 0 {
		t.Fatalf("sidebar top = %d, want 0 for top of window %q", top, geometry)
	}
	if width <= 0 {
		t.Fatalf("sidebar width = %d, want positive geometry %q", width, geometry)
	}
	if height <= 0 {
		t.Fatalf("sidebar height = %d, want positive geometry %q", height, geometry)
	}
}
