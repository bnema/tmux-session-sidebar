package uity

import lipgloss "charm.land/lipgloss/v2"

func (m SidebarModel) availableTreeHeight() int {
	if m.height <= 0 {
		return 0
	}
	height := m.height
	height-- // leading blank line above the tree.
	height -= len(m.statusBarLines(newSidebarStyles()))
	if m.statusLine() != "" {
		height -= 2 // blank separator plus status line.
	}
	if m.updateInProgress || m.message != "" {
		height -= 2 // blank separator plus message line.
	}
	return max(height, 0)
}

func (m SidebarModel) renderScrollableTree(styles sidebarStyles) []string {
	lines := m.renderTree(styles)
	if m.height <= 0 {
		return lines
	}
	viewportHeight := m.availableTreeHeight()
	if viewportHeight <= 0 {
		return nil
	}
	if len(lines) <= viewportHeight {
		return lines
	}
	scroll := m.normalizedTreeScroll(len(lines), viewportHeight, styles)
	return lines[scroll:min(scroll+viewportHeight, len(lines))]
}

func (m *SidebarModel) ensureTreeCursorVisible() {
	visible := m.selectableTreeItems()
	if len(visible) == 0 {
		m.cursor = 0
		m.treeScroll = 0
		return
	}
	m.cursor = min(max(m.cursor, 0), len(visible)-1)
	styles := newSidebarStyles()
	viewportHeight := m.availableTreeHeight()
	renderedHeight := m.renderedTreeLineCount(styles)
	if viewportHeight <= 0 || renderedHeight <= viewportHeight {
		m.treeScroll = 0
		return
	}
	selectedLine := m.selectedRenderedTreeLine(styles)
	if selectedLine < m.treeScroll {
		m.treeScroll = selectedLine
	}
	if selectedLine >= m.treeScroll+viewportHeight {
		m.treeScroll = selectedLine - viewportHeight + 1
	}
	m.treeScroll = m.normalizedTreeScroll(renderedHeight, viewportHeight, styles)
}

func (m SidebarModel) normalizedTreeScroll(renderedHeight int, viewportHeight int, styles sidebarStyles) int {
	if viewportHeight <= 0 || renderedHeight <= viewportHeight {
		return 0
	}
	scroll := min(max(m.treeScroll, 0), renderedHeight-viewportHeight)
	return m.snapTreeScrollToItemStart(scroll, styles)
}

func (m SidebarModel) snapTreeScrollToItemStart(scroll int, styles sidebarStyles) int {
	if scroll <= 0 {
		return 0
	}
	renderer := m.treeLineCounter(styles)
	line := 0
	for _, item := range m.visibleTreeItems() {
		next := line + renderer.renderedTreeItemLineCount(item)
		if scroll < next {
			return line
		}
		line = next
	}
	return line
}

func (m SidebarModel) renderedTreeLineCount(styles sidebarStyles) int {
	items := m.visibleTreeItems()
	if len(items) == 0 {
		return 1
	}
	renderer := m.treeLineCounter(styles)
	count := 0
	for _, item := range items {
		count += renderer.renderedTreeItemLineCount(item)
	}
	return count
}

func (m SidebarModel) selectedRenderedTreeLine(styles sidebarStyles) int {
	selectableIndex := 0
	line := 0
	renderer := m.treeLineCounter(styles)
	for _, item := range m.visibleTreeItems() {
		if isSelectableTreeKind(item.Kind) {
			if selectableIndex == m.cursor {
				return line
			}
			selectableIndex++
		}
		line += renderer.renderedTreeItemLineCount(item)
	}
	return 0
}

func (m SidebarModel) colorPickerOverlayY() int {
	styles := newSidebarStyles()
	paneHeight := lipgloss.Height(m.pinColorPicker.Render())
	selectedLine := m.selectedRenderedTreeLine(styles) - m.normalizedTreeScroll(m.renderedTreeLineCount(styles), m.availableTreeHeight(), styles)
	return min(selectedLine+2, max(m.height-paneHeight, 0))
}

func (m SidebarModel) treeLineCounter(styles sidebarStyles) treeRenderer {
	return treeRenderer{styles: styles, width: m.width, metadataIconMode: m.metadataIconMode}
}

func (r treeRenderer) renderedTreeItemLineCount(item TreeItem) int {
	if item.Kind == TreeRowSession && r.renderMetadata(item, false) != "" {
		return 2
	}
	return 1
}
