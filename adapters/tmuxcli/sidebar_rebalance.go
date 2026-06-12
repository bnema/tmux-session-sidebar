package tmuxcli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

type horizontalSidebarClosePlan struct {
	windowID              string
	representativePaneIDs []string
	workSpans             []int
}

type sidebarHorizontalGroup struct {
	Left                 int
	Width                int
	RepresentativePaneID string
	MinTop               int
	MaxBottom            int
	UniformWidth         bool
}

// tmux break-pane does not preserve top-level horizontal work-group
// proportions when it removes a full-height left sidebar. It gives the freed
// width to the adjacent group, which then becomes the hidden layout that a
// later reopen inherits. Capture the pre-close group spans so the hidden
// layout can be rebalanced back to those spans immediately after break-pane.
func (c Client) planHorizontalSidebarClose(ctx context.Context, windowID string, sidebarPaneID string) (*horizontalSidebarClosePlan, error) {
	windowID = strings.TrimSpace(windowID)
	sidebarPaneID = strings.TrimSpace(sidebarPaneID)
	if windowID == "" || sidebarPaneID == "" {
		return nil, nil
	}
	windowWidth, windowHeight, err := c.windowDimensions(ctx, windowID)
	if err != nil {
		return nil, err
	}
	panes, err := c.listSidebarRebalancePanes(ctx, windowID)
	if err != nil {
		return nil, err
	}
	if len(panes) < 3 {
		return nil, nil
	}
	sidebar, work := splitSidebarAndWorkPanes(panes, sidebarPaneID)
	if sidebar == nil || !sidebar.Sidebar || sidebar.Left != 0 || sidebar.Top != 0 || sidebar.Height != windowHeight {
		return nil, nil
	}
	groups := groupHorizontalPanes(work)
	if len(groups) < 2 {
		return nil, nil
	}
	spans := make([]int, 0, len(groups))
	reps := make([]string, 0, len(groups))
	for i, group := range groups {
		if !group.UniformWidth || group.MinTop != 0 || group.MaxBottom != windowHeight {
			return nil, nil
		}
		nextLeft := windowWidth
		if i+1 < len(groups) {
			nextLeft = groups[i+1].Left
		}
		span := nextLeft - group.Left
		if span <= 0 || strings.TrimSpace(group.RepresentativePaneID) == "" {
			return nil, nil
		}
		spans = append(spans, span)
		reps = append(reps, group.RepresentativePaneID)
	}
	return &horizontalSidebarClosePlan{windowID: windowID, representativePaneIDs: reps, workSpans: spans}, nil
}

func (c Client) rebalanceHorizontalSidebarCloseBestEffort(ctx context.Context, plan *horizontalSidebarClosePlan) {
	if plan == nil {
		return
	}
	panes, err := c.listSidebarRebalancePanes(ctx, plan.windowID)
	if err != nil {
		return
	}
	for _, pane := range panes {
		if pane.Sidebar {
			return
		}
	}
	groups := groupHorizontalPanes(panes)
	if len(groups) != len(plan.representativePaneIDs) {
		return
	}
	groupByRep := make(map[string]sidebarHorizontalGroup, len(groups))
	totalWidth := 0
	for _, group := range groups {
		if !group.UniformWidth {
			return
		}
		groupByRep[group.RepresentativePaneID] = group
		totalWidth += group.Width
	}
	targetWidths := proportionalWidths(plan.workSpans, totalWidth)
	for i := 0; i < len(plan.representativePaneIDs)-1; i++ {
		repID := plan.representativePaneIDs[i]
		group, ok := groupByRep[repID]
		if !ok || targetWidths[i] <= 0 {
			return
		}
		if group.Width == targetWidths[i] {
			continue
		}
		_ = c.resizePaneWidth(ctx, repID, strconv.Itoa(targetWidths[i]))
	}
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
