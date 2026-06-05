package uity

import sidebarlayout "github.com/bnema/tmux-session-sidebar/core/sidebar"

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
	setCategorySessionsExpanded(m.treeItems, categoryID, expanded)
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
	for categoryID := range expanded {
		setCategorySessionsExpanded(items, categoryID, true)
	}
}

func setCategorySessionsExpanded(items []TreeItem, categoryID string, expanded bool) {
	sessionIndex := 0
	for i := range items {
		if items[i].CategoryID != categoryID {
			continue
		}
		switch items[i].Kind {
		case TreeRowMore:
			items[i].MoreExpanded = expanded
		case TreeRowSession:
			sessionIndex++
			items[i].OverflowHidden = !expanded && sessionIndex > sidebarlayout.CategorySessionPreviewLimit
		}
	}
}
