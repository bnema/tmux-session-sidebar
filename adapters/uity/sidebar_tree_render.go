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
	m.items = sessionItemsFromTree(m.treeItems)
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
	return m.visibleTreeItems()
}

func isSelectableTreeKind(kind TreeRowKind) bool {
	switch kind {
	case TreeRowSession, TreeRowCategory, TreeRowSeparator, TreeRowSpacer:
		return true
	default:
		return false
	}
}

func (m SidebarModel) visibleTreeItems() []TreeItem {
	if m.filter == "" {
		return m.treeItems
	}
	visibleSessions := m.visibleItems()
	visibleSet := make(map[string]bool, len(visibleSessions))
	for _, item := range visibleSessions {
		visibleSet[item.Name] = true
	}
	matchedSessions := 0
	filtered := make([]TreeItem, 0, len(m.treeItems))
	for _, item := range m.treeItems {
		if item.Kind == TreeRowSession {
			if !visibleSet[item.Session.Name] {
				continue
			}
			matchedSessions++
		}
		filtered = append(filtered, item)
	}
	if matchedSessions == 0 {
		return nil
	}
	return filtered
}
