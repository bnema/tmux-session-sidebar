package tmuxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

const formatSidebarRebalancePane = "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}\t#{?@session-sidebar-pane,1,0}"

type sidebarRebalancePane struct {
	PaneID  string
	Left    int
	Top     int
	Width   int
	Height  int
	Sidebar bool
}

type sidebarWorkWeightMode int

const (
	sidebarWorkWeightByGroupWidth sidebarWorkWeightMode = iota
	sidebarWorkWeightByGroupSpan
)

type sidebarWorkWeight struct {
	RepresentativePaneID string
	Weight               int
}

type sidebarOpenWorkBaseline struct {
	RepresentativePaneIDs []string `json:"representativePaneIDs"`
	WorkWidths            []int    `json:"workWidths"`
}

type sidebarHorizontalGroup struct {
	Left                 int
	Width                int
	RepresentativePaneID string
	MinTop               int
	MaxBottom            int
	UniformWidth         bool
}

func (c Client) CaptureAttachedSidebarWidthBaseline(ctx context.Context, windowID string, paneID string, width string) error {
	windowID, paneID, width, err := c.normalizeAttachedSidebarResizeInputs(ctx, windowID, paneID, width)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if windowID == "" || paneID == "" {
		c.resizeDebug("resize-baseline-capture-skip", []ports.LogField{{Key: "reason", Value: "missing-target"}, {Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
		return nil
	}
	c.resizeDebug("resize-baseline-capture-start", []ports.LogField{{Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
	active, err := c.sidebarResizeSyncActive(ctx, windowID)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if active {
		c.resizeDebug("resize-baseline-capture-skip", []ports.LogField{{Key: "reason", Value: "resize-sync-active"}, {Key: "window", Value: windowID}, {Key: "pane", Value: paneID}})
		return nil
	}
	weights, err := c.captureSidebarWorkWeights(ctx, windowID, paneID, width, sidebarWorkWeightByGroupWidth)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	return c.saveSidebarOpenWorkBaseline(ctx, windowID, weights)
}

func (c Client) SyncAttachedSidebarWidth(ctx context.Context, windowID string, paneID string, width string) error {
	windowID, paneID, width, err := c.normalizeAttachedSidebarResizeInputs(ctx, windowID, paneID, width)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	if windowID == "" || paneID == "" {
		c.resizeDebug("resize-sync-skip", []ports.LogField{{Key: "reason", Value: "missing-target"}, {Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
		return nil
	}
	c.resizeDebug("resize-sync-start", []ports.LogField{{Key: "window", Value: windowID}, {Key: "pane", Value: paneID}, {Key: "width", Value: width}})
	weights, err := c.loadSidebarOpenWorkBaseline(ctx, windowID)
	if err != nil {
		if isTmuxTargetGone(err) {
			return nil
		}
		return err
	}
	c.setSidebarResizeSyncActiveBestEffort(ctx, windowID, true)
	defer c.setSidebarResizeSyncActiveBestEffort(ctx, windowID, false)
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdResizePane, "-t", paneID, "-x", width})
	if err != nil {
		wrapped := wrapTmuxError(result, err)
		if isTmuxTargetGone(wrapped) {
			return nil
		}
		return wrapped
	}
	c.resizeDebug("resize-sidebar-pane", []ports.LogField{{Key: "pane", Value: paneID}, {Key: "width", Value: width}})
	c.applySidebarWorkWeightsBestEffort(ctx, windowID, paneID, weights, true)
	return nil
}

func (c Client) normalizeAttachedSidebarResizeInputs(ctx context.Context, windowID string, paneID string, width string) (string, string, string, error) {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return "", "", "", nil
	}
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		resolvedWindowID, err := c.WindowID(ctx, paneID)
		if err != nil {
			return "", "", "", err
		}
		windowID = strings.TrimSpace(resolvedWindowID)
	}
	width = strings.TrimSpace(width)
	if width == "" {
		width = "30"
	}
	return windowID, paneID, width, nil
}

func parseSidebarWidthCells(width string) (int, bool) {
	width = strings.TrimSpace(width)
	if width == "" {
		width = "30"
	}
	parsed, err := strconv.Atoi(width)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func (c Client) captureSidebarWorkWeights(ctx context.Context, windowID string, sidebarPaneID string, width string, mode sidebarWorkWeightMode) ([]sidebarWorkWeight, error) {
	expectedSidebarWidth := 0
	if mode == sidebarWorkWeightByGroupWidth {
		parsedWidth, ok := parseSidebarWidthCells(width)
		if !ok {
			c.resizeDebug("resize-capture-weights-skip", []ports.LogField{{Key: "reason", Value: "invalid-sidebar-width"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "width", Value: width}})
			return nil, nil
		}
		expectedSidebarWidth = parsedWidth
	}
	groups, windowWidth, err := c.sidebarHorizontalWorkGroups(ctx, windowID, sidebarPaneID, true, expectedSidebarWidth)
	if err != nil || len(groups) == 0 {
		if err == nil {
			c.resizeDebug("resize-capture-weights-skip", []ports.LogField{{Key: "reason", Value: "no-valid-work-groups"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}})
		}
		return nil, err
	}
	weights := make([]sidebarWorkWeight, 0, len(groups))
	for i, group := range groups {
		weight := group.Width
		if mode == sidebarWorkWeightByGroupSpan {
			nextLeft := windowWidth
			if i+1 < len(groups) {
				nextLeft = groups[i+1].Left
			}
			weight = nextLeft - group.Left
		}
		if strings.TrimSpace(group.RepresentativePaneID) == "" || weight <= 0 {
			c.resizeDebug("resize-capture-weights-skip", []ports.LogField{{Key: "reason", Value: "invalid-group-weight"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "group", Value: formatSidebarHorizontalGroup(group)}, {Key: "weight", Value: weight}})
			return nil, nil
		}
		weights = append(weights, sidebarWorkWeight{RepresentativePaneID: group.RepresentativePaneID, Weight: weight})
	}
	c.resizeDebug("resize-captured-work-weights", []ports.LogField{{Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "mode", Value: mode.String()}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}})
	return weights, nil
}

func (c Client) saveSidebarOpenWorkBaseline(ctx context.Context, windowID string, weights []sidebarWorkWeight) error {
	baseline := sidebarOpenWorkBaseline{RepresentativePaneIDs: make([]string, 0, len(weights)), WorkWidths: make([]int, 0, len(weights))}
	for _, weight := range weights {
		if strings.TrimSpace(weight.RepresentativePaneID) == "" || weight.Weight <= 0 {
			c.resizeDebug("resize-baseline-clear", []ports.LogField{{Key: "window", Value: windowID}, {Key: "reason", Value: "invalid-weight"}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}})
			return c.clearWindowOptionValue(ctx, windowID, optionSidebarOpenWorkBaseline)
		}
		baseline.RepresentativePaneIDs = append(baseline.RepresentativePaneIDs, weight.RepresentativePaneID)
		baseline.WorkWidths = append(baseline.WorkWidths, weight.Weight)
	}
	if len(baseline.RepresentativePaneIDs) < 2 {
		c.resizeDebug("resize-baseline-clear", []ports.LogField{{Key: "window", Value: windowID}, {Key: "reason", Value: "too-few-work-groups"}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}})
		return c.clearWindowOptionValue(ctx, windowID, optionSidebarOpenWorkBaseline)
	}
	encoded, err := json.Marshal(baseline)
	if err != nil {
		return err
	}
	c.resizeDebug("resize-baseline-save", []ports.LogField{{Key: "window", Value: windowID}, {Key: "representatives", Value: baseline.RepresentativePaneIDs}, {Key: "widths", Value: baseline.WorkWidths}})
	return c.setWindowOptionValue(ctx, windowID, optionSidebarOpenWorkBaseline, string(encoded))
}

func (c Client) loadSidebarOpenWorkBaseline(ctx context.Context, windowID string) ([]sidebarWorkWeight, error) {
	raw, err := c.windowOptionValue(ctx, windowID, optionSidebarOpenWorkBaseline)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		c.resizeDebug("resize-baseline-empty", []ports.LogField{{Key: "window", Value: windowID}})
		return nil, nil
	}
	var baseline sidebarOpenWorkBaseline
	if err := json.Unmarshal([]byte(raw), &baseline); err != nil {
		c.resizeDebug("resize-baseline-invalid", []ports.LogField{{Key: "window", Value: windowID}, {Key: "reason", Value: "decode"}, {Key: "raw", Value: raw}, {Key: "error", Value: err.Error()}})
		return nil, nil
	}
	if len(baseline.RepresentativePaneIDs) < 2 || len(baseline.RepresentativePaneIDs) != len(baseline.WorkWidths) {
		c.resizeDebug("resize-baseline-invalid", []ports.LogField{{Key: "window", Value: windowID}, {Key: "reason", Value: "shape"}, {Key: "representatives", Value: baseline.RepresentativePaneIDs}, {Key: "widths", Value: baseline.WorkWidths}})
		return nil, nil
	}
	weights := make([]sidebarWorkWeight, 0, len(baseline.RepresentativePaneIDs))
	for i, repID := range baseline.RepresentativePaneIDs {
		repID = strings.TrimSpace(repID)
		if repID == "" || baseline.WorkWidths[i] <= 0 {
			c.resizeDebug("resize-baseline-invalid", []ports.LogField{{Key: "window", Value: windowID}, {Key: "reason", Value: "invalid-entry"}, {Key: "representatives", Value: baseline.RepresentativePaneIDs}, {Key: "widths", Value: baseline.WorkWidths}})
			return nil, nil
		}
		weights = append(weights, sidebarWorkWeight{RepresentativePaneID: repID, Weight: baseline.WorkWidths[i]})
	}
	c.resizeDebug("resize-baseline-loaded", []ports.LogField{{Key: "window", Value: windowID}, {Key: "representatives", Value: baseline.RepresentativePaneIDs}, {Key: "widths", Value: baseline.WorkWidths}})
	return weights, nil
}

func (c Client) sidebarResizeSyncActive(ctx context.Context, windowID string) (bool, error) {
	value, err := c.windowOptionValue(ctx, windowID, optionSidebarResizeSyncActive)
	if err != nil {
		return false, err
	}
	return parseTmuxBool(value), nil
}

func (c Client) setSidebarResizeSyncActiveBestEffort(ctx context.Context, windowID string, active bool) {
	if active {
		_ = c.setWindowOptionValue(ctx, windowID, optionSidebarResizeSyncActive, "1")
		return
	}
	_ = c.clearWindowOptionValue(ctx, windowID, optionSidebarResizeSyncActive)
}

func (c Client) applySidebarWorkWeightsBestEffort(ctx context.Context, windowID string, sidebarPaneID string, weights []sidebarWorkWeight, requireSidebar bool) {
	if len(weights) < 2 {
		c.resizeDebug("resize-work-weights-skip", []ports.LogField{{Key: "reason", Value: "too-few-baseline-weights"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}})
		return
	}
	groups, _, err := c.sidebarHorizontalWorkGroups(ctx, windowID, sidebarPaneID, requireSidebar, 0)
	if err != nil || len(groups) != len(weights) {
		if err == nil {
			c.resizeDebug("resize-work-weights-skip", []ports.LogField{{Key: "reason", Value: "group-count-mismatch"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}, {Key: "groups", Value: formatSidebarHorizontalGroups(groups)}})
		}
		return
	}
	groupByRep := make(map[string]sidebarHorizontalGroup, len(groups))
	totalWidth := 0
	for _, group := range groups {
		groupByRep[group.RepresentativePaneID] = group
		totalWidth += group.Width
	}
	alignedGroups := make([]sidebarHorizontalGroup, len(weights))
	targetWeights := make([]int, len(weights))
	for i, weight := range weights {
		representativePaneID := strings.TrimSpace(weight.RepresentativePaneID)
		if representativePaneID == "" || weight.Weight <= 0 {
			c.resizeDebug("resize-work-weights-skip", []ports.LogField{{Key: "reason", Value: "invalid-baseline-weight"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}})
			return
		}
		group, ok := groupByRep[representativePaneID]
		if !ok {
			c.resizeDebug("resize-work-weights-skip", []ports.LogField{{Key: "reason", Value: "missing-current-group"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "representative", Value: representativePaneID}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}, {Key: "groups", Value: formatSidebarHorizontalGroups(groups)}})
			return
		}
		alignedGroups[i] = group
		targetWeights[i] = weight.Weight
	}
	targetWidths := proportionalWidths(targetWeights, totalWidth)
	for _, targetWidth := range targetWidths {
		if targetWidth <= 0 {
			c.resizeDebug("resize-work-weights-skip", []ports.LogField{{Key: "reason", Value: "invalid-target-width"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}, {Key: "target_widths", Value: targetWidths}})
			return
		}
	}
	c.resizeDebug("resize-work-weights", []ports.LogField{{Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "total_width", Value: totalWidth}, {Key: "weights", Value: formatSidebarWorkWeights(weights)}, {Key: "target_widths", Value: targetWidths}})
	for i := 0; i < len(alignedGroups)-1; i++ {
		group := alignedGroups[i]
		if group.Width == targetWidths[i] {
			c.resizeDebug("resize-work-pane-skip", []ports.LogField{{Key: "pane", Value: group.RepresentativePaneID}, {Key: "reason", Value: "already-target-width"}, {Key: "width", Value: group.Width}})
			continue
		}
		c.resizeDebug("resize-work-pane", []ports.LogField{{Key: "pane", Value: group.RepresentativePaneID}, {Key: "from", Value: group.Width}, {Key: "to", Value: targetWidths[i]}})
		if err := c.resizePaneWidth(ctx, group.RepresentativePaneID, strconv.Itoa(targetWidths[i])); err != nil {
			c.resizeDebug("resize-work-pane-error", []ports.LogField{{Key: "pane", Value: group.RepresentativePaneID}, {Key: "from", Value: group.Width}, {Key: "to", Value: targetWidths[i]}, {Key: "error", Value: err.Error()}})
		}
	}
}

func (c Client) sidebarHorizontalWorkGroups(ctx context.Context, windowID string, sidebarPaneID string, requireSidebar bool, expectedSidebarWidth int) ([]sidebarHorizontalGroup, int, error) {
	windowWidth, windowHeight, err := c.windowDimensions(ctx, windowID)
	if err != nil {
		return nil, 0, err
	}
	panes, err := c.listSidebarRebalancePanes(ctx, windowID)
	if err != nil {
		return nil, 0, err
	}
	var work []sidebarRebalancePane
	if requireSidebar {
		if len(panes) < 3 {
			c.resizeDebug("resize-work-groups-skip", []ports.LogField{{Key: "reason", Value: "too-few-panes"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "require_sidebar", Value: requireSidebar}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}})
			return nil, 0, nil
		}
		sidebar, nonSidebar := splitSidebarAndWorkPanes(panes, sidebarPaneID)
		if sidebar == nil || !sidebar.Sidebar || sidebar.Left != 0 || sidebar.Top != 0 || sidebar.Height != windowHeight {
			c.resizeDebug("resize-work-groups-skip", []ports.LogField{{Key: "reason", Value: "invalid-sidebar-shape"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "require_sidebar", Value: requireSidebar}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}})
			return nil, 0, nil
		}
		if expectedSidebarWidth > 0 && sidebar.Width != expectedSidebarWidth {
			c.resizeDebug("resize-work-groups-skip", []ports.LogField{{Key: "reason", Value: "sidebar-width-mismatch"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "actual_sidebar_width", Value: sidebar.Width}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}})
			return nil, 0, nil
		}
		work = nonSidebar
	} else {
		for _, pane := range panes {
			if pane.Sidebar {
				c.resizeDebug("resize-work-groups-skip", []ports.LogField{{Key: "reason", Value: "unexpected-sidebar-pane"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "require_sidebar", Value: requireSidebar}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}})
				return nil, 0, nil
			}
		}
		work = panes
	}
	groups := groupHorizontalPanes(work)
	if len(groups) < 2 || !validSidebarHorizontalGroups(groups, windowHeight) {
		c.resizeDebug("resize-work-groups-skip", []ports.LogField{{Key: "reason", Value: "invalid-work-groups"}, {Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "require_sidebar", Value: requireSidebar}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}, {Key: "groups", Value: formatSidebarHorizontalGroups(groups)}})
		return nil, 0, nil
	}
	c.resizeDebug("resize-work-groups", []ports.LogField{{Key: "window", Value: windowID}, {Key: "sidebar", Value: sidebarPaneID}, {Key: "require_sidebar", Value: requireSidebar}, {Key: "expected_sidebar_width", Value: expectedSidebarWidth}, {Key: "window_width", Value: windowWidth}, {Key: "window_height", Value: windowHeight}, {Key: "panes", Value: formatSidebarRebalancePanes(panes)}, {Key: "groups", Value: formatSidebarHorizontalGroups(groups)}})
	return groups, windowWidth, nil
}

func validSidebarHorizontalGroups(groups []sidebarHorizontalGroup, windowHeight int) bool {
	for _, group := range groups {
		if !group.UniformWidth || group.MinTop != 0 || group.MaxBottom != windowHeight || strings.TrimSpace(group.RepresentativePaneID) == "" || group.Width <= 0 {
			return false
		}
	}
	return true
}

func splitSidebarAndWorkPanes(panes []sidebarRebalancePane, sidebarPaneID string) (*sidebarRebalancePane, []sidebarRebalancePane) {
	work := make([]sidebarRebalancePane, 0, len(panes))
	var sidebar *sidebarRebalancePane
	for i := range panes {
		pane := panes[i]
		if pane.PaneID == sidebarPaneID {
			paneCopy := pane
			sidebar = &paneCopy
			continue
		}
		work = append(work, pane)
	}
	return sidebar, work
}

// groupHorizontalPanes collapses panes that share the same left edge into one
// top-level horizontal group. A vertically split stack therefore acts as one
// work group whose width should stay in proportion to its sibling groups.
func groupHorizontalPanes(panes []sidebarRebalancePane) []sidebarHorizontalGroup {
	if len(panes) == 0 {
		return nil
	}
	type aggregate struct {
		left                 int
		width                int
		representativePaneID string
		representativeTop    int
		minTop               int
		maxBottom            int
		uniformWidth         bool
	}
	byLeft := map[int]aggregate{}
	for _, pane := range panes {
		agg, ok := byLeft[pane.Left]
		if !ok {
			byLeft[pane.Left] = aggregate{left: pane.Left, width: pane.Width, representativePaneID: pane.PaneID, representativeTop: pane.Top, minTop: pane.Top, maxBottom: pane.Top + pane.Height, uniformWidth: true}
			continue
		}
		if pane.Width != agg.width {
			agg.uniformWidth = false
		}
		if pane.Width > agg.width {
			agg.width = pane.Width
		}
		if pane.Top < agg.minTop {
			agg.minTop = pane.Top
		}
		if bottom := pane.Top + pane.Height; bottom > agg.maxBottom {
			agg.maxBottom = bottom
		}
		if pane.Top < agg.representativeTop {
			agg.representativeTop = pane.Top
			agg.representativePaneID = pane.PaneID
		}
		byLeft[pane.Left] = agg
	}
	lefts := make([]int, 0, len(byLeft))
	for left := range byLeft {
		lefts = append(lefts, left)
	}
	sort.Ints(lefts)
	groups := make([]sidebarHorizontalGroup, 0, len(lefts))
	for _, left := range lefts {
		agg := byLeft[left]
		groups = append(groups, sidebarHorizontalGroup{Left: agg.left, Width: agg.width, RepresentativePaneID: agg.representativePaneID, MinTop: agg.minTop, MaxBottom: agg.maxBottom, UniformWidth: agg.uniformWidth})
	}
	return groups
}

func (c Client) listSidebarRebalancePanes(ctx context.Context, windowID string) ([]sidebarRebalancePane, error) {
	result, err := c.Process.Exec(ctx, tmuxBinary, []string{cmdListPanes, "-t", windowID, "-F", formatSidebarRebalancePane})
	if err != nil {
		return nil, wrapTmuxError(result, err)
	}
	panes := make([]sidebarRebalancePane, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			return nil, fmt.Errorf("expected 6 tab-separated fields, got %d: %q", len(fields), line)
		}
		left, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, err
		}
		top, err := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err != nil {
			return nil, err
		}
		width, err := strconv.Atoi(strings.TrimSpace(fields[3]))
		if err != nil {
			return nil, err
		}
		height, err := strconv.Atoi(strings.TrimSpace(fields[4]))
		if err != nil {
			return nil, err
		}
		panes = append(panes, sidebarRebalancePane{
			PaneID:  strings.TrimSpace(fields[0]),
			Left:    left,
			Top:     top,
			Width:   width,
			Height:  height,
			Sidebar: parseTmuxBool(strings.TrimSpace(fields[5])),
		})
	}
	return panes, nil
}

func (c Client) windowDimensions(ctx context.Context, windowID string) (int, int, error) {
	out, err := c.displayTarget(ctx, windowID, "#{window_width}\t#{window_height}")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Split(out, "\t")
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("expected 2 tab-separated fields for window dimensions, got %d: %q", len(fields), out)
	}
	width, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func (c Client) resizeDebug(msg string, fields []ports.LogField) {
	if c.Logger == nil {
		return
	}
	c.Logger.Debug(msg, fields)
}

func (m sidebarWorkWeightMode) String() string {
	switch m {
	case sidebarWorkWeightByGroupWidth:
		return "group-width"
	case sidebarWorkWeightByGroupSpan:
		return "group-span"
	default:
		return "unknown"
	}
}

type sidebarWorkWeightsLog []sidebarWorkWeight

type sidebarRebalancePanesLog []sidebarRebalancePane

type sidebarHorizontalGroupLog sidebarHorizontalGroup

type sidebarHorizontalGroupsLog []sidebarHorizontalGroup

func formatSidebarWorkWeights(weights []sidebarWorkWeight) fmt.Stringer {
	return sidebarWorkWeightsLog(weights)
}

func (weights sidebarWorkWeightsLog) String() string {
	parts := make([]string, 0, len(weights))
	for _, weight := range weights {
		parts = append(parts, fmt.Sprintf("%s=%d", weight.RepresentativePaneID, weight.Weight))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func formatSidebarRebalancePanes(panes []sidebarRebalancePane) fmt.Stringer {
	return sidebarRebalancePanesLog(panes)
}

func (panes sidebarRebalancePanesLog) String() string {
	parts := make([]string, 0, len(panes))
	for _, pane := range panes {
		parts = append(parts, fmt.Sprintf("%s@%d,%d %dx%d sidebar=%t", pane.PaneID, pane.Left, pane.Top, pane.Width, pane.Height, pane.Sidebar))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func formatSidebarHorizontalGroup(group sidebarHorizontalGroup) fmt.Stringer {
	return sidebarHorizontalGroupLog(group)
}

func (group sidebarHorizontalGroupLog) String() string {
	return fmt.Sprintf("%s left=%d width=%d top=%d bottom=%d uniform=%t", group.RepresentativePaneID, group.Left, group.Width, group.MinTop, group.MaxBottom, group.UniformWidth)
}

func formatSidebarHorizontalGroups(groups []sidebarHorizontalGroup) fmt.Stringer {
	return sidebarHorizontalGroupsLog(groups)
}

func (groups sidebarHorizontalGroupsLog) String() string {
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, sidebarHorizontalGroupLog(group).String())
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func proportionalWidths(weights []int, total int) []int {
	if len(weights) == 0 || total <= 0 {
		return nil
	}
	sum := 0
	for _, weight := range weights {
		if weight > 0 {
			sum += weight
		}
	}
	if sum == 0 {
		return nil
	}
	widths := make([]int, len(weights))
	type remainder struct {
		index int
		rem   int64
	}
	remainders := make([]remainder, 0, len(weights))
	assigned := 0
	total64 := int64(total)
	sum64 := int64(sum)
	for i, weight := range weights {
		if weight <= 0 {
			continue
		}
		numerator64 := int64(weight) * total64
		widths[i] = int(numerator64 / sum64)
		assigned += widths[i]
		remainders = append(remainders, remainder{index: i, rem: numerator64 % sum64})
	}
	sort.SliceStable(remainders, func(i, j int) bool { return remainders[i].rem > remainders[j].rem })
	remaining := min(total-assigned, len(remainders))
	for j := range remaining {
		widths[remainders[j].index]++
	}
	return widths
}
