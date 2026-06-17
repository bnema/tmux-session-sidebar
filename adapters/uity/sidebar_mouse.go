package uity

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *SidebarModel) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return *m, nil
	}
	m.focused = true
	if m.mode != ModeBrowse || m.showHelp {
		return *m, nil
	}
	_, selectableIndex, ok := m.sessionNameAtMouse(mouse.X, mouse.Y)
	if !ok {
		return *m, nil
	}
	m.cursor = selectableIndex
	m.activateSelected()
	return m.finishInteractiveUpdate()
}

func (m SidebarModel) sessionNameAtMouse(x int, y int) (TreeItem, int, bool) {
	if x < 0 || y < 0 {
		return TreeItem{}, 0, false
	}
	// Render() keeps one blank/padded line above the tree.
	treeY := y - 1
	if treeY < 0 {
		return TreeItem{}, 0, false
	}
	styles := newSidebarStylesForAppearance(m.appearance)
	viewportHeight := m.availableTreeHeight()
	if m.height > 0 && (viewportHeight <= 0 || treeY >= viewportHeight) {
		return TreeItem{}, 0, false
	}
	renderer := m.treeLineCounter(styles)
	renderedHeight := m.renderedTreeLineCount(styles)
	scroll := 0
	if m.height > 0 {
		scroll = m.normalizedTreeScroll(renderedHeight, viewportHeight, styles)
	}
	targetLine := scroll + treeY
	line := 0
	selectableIndex := 0
	for _, item := range m.visibleTreeItems() {
		selectable := isSelectableTreeKind(item.Kind)
		if item.Kind == TreeRowSession && targetLine == line && m.mouseXHitsSessionName(x, item) {
			return item, selectableIndex, true
		}
		line += renderer.renderedTreeItemLineCount(item)
		if selectable {
			selectableIndex++
		}
	}
	return TreeItem{}, 0, false
}

func (m SidebarModel) mouseXHitsSessionName(x int, item TreeItem) bool {
	start, end, ok := m.sessionNameClickBounds(item)
	return ok && x >= start && x < end
}

func (m SidebarModel) sessionNameClickBounds(item TreeItem) (int, int, bool) {
	if item.Kind != TreeRowSession {
		return 0, 0, false
	}
	currentMarker := ""
	if item.Session.Current {
		currentMarker = "┃"
	}
	slot := slotPrefix(item.Slot)
	marker := treeSessionMarker(item.Session)
	name := treeRenderer{width: m.width, metadataIconMode: m.metadataIconMode}.fitSessionName(item, currentMarker, slot, marker)
	if name == "" {
		return 0, 0, false
	}
	prefixWidth := 0
	if currentMarker == "" {
		prefixWidth++ // leading body space before non-current session names.
	}
	if trimmedSlot := strings.TrimSpace(slot); trimmedSlot != "" {
		prefixWidth += metadataDisplayWidth(trimmedSlot) + 1
	}
	if trimmedMarker := strings.TrimSpace(marker); trimmedMarker != "" {
		prefixWidth += metadataDisplayWidth(trimmedMarker) + 1
	}
	start := 1 // horizontal padding added by padSidebarContentLines.
	start += metadataDisplayWidth(treeBranch(item))
	start += metadataDisplayWidth(currentMarker)
	start += prefixWidth
	end := start + metadataDisplayWidth(name)
	return start, end, end > start
}
