package uity

import "strings"

func (m SidebarModel) renderTree(styles sidebarStyles) []string {
	return treeRenderer{
		styles:           styles,
		width:            m.width,
		metadataIconMode: m.metadataIconMode,
		animationStyle:   m.attentionAnimationStyle,
		animationFrame:   m.attentionAnimationFrame,
		cursor:           m.cursor,
	}.Render(m.visibleTreeItems())
}

func (m *SidebarModel) reloadTreeItems() bool {
	if m.actions.ReloadTreeItems == nil {
		return false
	}
	expanded := expandedCategorySet(m.treeItems)
	next := m.actions.ReloadTreeItems()
	if next == nil {
		return false
	}
	preserveExpandedCategories(next, expanded)
	m.treeItems = next
	if m.cursor >= len(m.selectableTreeItems()) {
		m.cursor = max(len(m.selectableTreeItems())-1, 0)
	}
	return true
}

func (m *SidebarModel) setLocalCategorySessionsExpanded(categoryID string, expanded bool) {
	for i := range m.treeItems {
		if m.treeItems[i].CategoryID != categoryID {
			continue
		}
		if m.treeItems[i].Kind == TreeRowMore {
			m.treeItems[i].MoreExpanded = expanded
		}
		if expanded && m.treeItems[i].Kind == TreeRowSession {
			m.treeItems[i].OverflowHidden = false
		}
	}
}

func expandedCategorySet(items []TreeItem) map[string]bool {
	expanded := map[string]bool{}
	for _, item := range items {
		if item.Kind == TreeRowMore && item.MoreExpanded && item.CategoryID != "" {
			expanded[item.CategoryID] = true
		}
	}
	return expanded
}

func preserveExpandedCategories(items []TreeItem, expanded map[string]bool) {
	if len(expanded) == 0 {
		return
	}
	for i := range items {
		if !expanded[items[i].CategoryID] {
			continue
		}
		if items[i].Kind == TreeRowMore {
			items[i].MoreExpanded = true
		}
		if items[i].Kind == TreeRowSession {
			items[i].OverflowHidden = false
		}
	}
}

func (m SidebarModel) selectedTreeItem() (TreeItem, bool) {
	selectable := m.selectableTreeItems()
	if len(selectable) == 0 || m.cursor >= len(selectable) {
		return TreeItem{}, false
	}
	return selectable[m.cursor], true
}

func (m *SidebarModel) selectTreeItem(id string) {
	selectable := m.selectableTreeItems()
	for i, item := range selectable {
		if item.ID == id {
			m.cursor = i
			return
		}
	}
	if _, sessionName, ok := strings.Cut(id, "/session:"); ok {
		for i, item := range selectable {
			if item.Kind == TreeRowSession && item.Session.Name == sessionName {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = 0
}

func (m SidebarModel) selectableTreeItems() []TreeItem {
	items := m.visibleTreeItems()
	selectable := make([]TreeItem, 0, len(items))
	for _, item := range items {
		if isSelectableTreeKind(item.Kind) {
			selectable = append(selectable, item)
		}
	}
	return selectable
}

func isSelectableTreeKind(kind TreeRowKind) bool {
	switch kind {
	case TreeRowSession, TreeRowCategory, TreeRowSeparator, TreeRowSpacer, TreeRowMore:
		return true
	default:
		return false
	}
}

func (m SidebarModel) visibleTreeItems() []TreeItem {
	if m.filter == "" {
		visible := make([]TreeItem, 0, len(m.treeItems))
		for _, item := range m.treeItems {
			if item.OverflowHidden {
				continue
			}
			visible = append(visible, item)
		}
		return visible
	}
	filter := strings.ToLower(m.filter)
	filtered := make([]TreeItem, 0, len(m.treeItems))
	var category TreeItem
	categoryAdded := false
	lastSessionIndex := -1
	for _, item := range m.treeItems {
		switch item.Kind {
		case TreeRowCategory:
			category = item
			categoryAdded = false
			lastSessionIndex = -1
		case TreeRowSession:
			if !strings.Contains(strings.ToLower(item.Session.Name), filter) {
				continue
			}
			if category.ID != "" && !categoryAdded {
				filtered = append(filtered, category)
				categoryAdded = true
			}
			if lastSessionIndex >= 0 {
				filtered[lastSessionIndex].LastChild = false
			}
			item.OverflowHidden = false
			item.LastChild = true
			filtered = append(filtered, item)
			lastSessionIndex = len(filtered) - 1
		}
	}
	return filtered
}
