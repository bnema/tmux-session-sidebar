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

func (m *SidebarModel) reloadTreeItems() {
	if m.actions.ReloadTreeItems == nil {
		return
	}
	next := m.actions.ReloadTreeItems()
	if next == nil {
		return
	}
	m.treeItems = next
	if m.cursor >= len(m.selectableTreeItems()) {
		m.cursor = max(len(m.selectableTreeItems())-1, 0)
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
	for _, item := range m.treeItems {
		switch item.Kind {
		case TreeRowCategory:
			category = item
			categoryAdded = false
		case TreeRowSession:
			if !strings.Contains(strings.ToLower(item.Session.Name), filter) {
				continue
			}
			if category.ID != "" && !categoryAdded {
				filtered = append(filtered, category)
				categoryAdded = true
			}
			item.OverflowHidden = false
			filtered = append(filtered, item)
		}
	}
	return filtered
}
