package uity

const (
	newItemCategory  = "category"
	newItemSpacer    = "spacer"
	newItemSeparator = "separator"
)

func newItemMenuItems() []ProjectItem {
	return []ProjectItem{
		{Name: "New category", Path: newItemCategory},
		{Name: "New spacer", Path: newItemSpacer},
		{Name: "New separator", Path: newItemSeparator},
	}
}

func (m *SidebarModel) createSelectedNewItem() {
	visible := m.visibleProjects()
	if len(visible) == 0 || m.projectCursor >= len(visible) {
		return
	}
	choice := visible[m.projectCursor]
	ok := false
	switch choice.Path {
	case newItemCategory:
		if m.actions.CreateCategory != nil {
			ok = m.actions.CreateCategory("New category")
		}
	case newItemSpacer:
		if m.actions.CreateSpacer != nil {
			ok = m.actions.CreateSpacer()
		}
	case newItemSeparator:
		if m.actions.CreateSeparator != nil {
			ok = m.actions.CreateSeparator()
		}
	}
	if ok {
		m.mode = ModeBrowse
		m.projectCursor = 0
		m.projectFilter = ""
		m.reloadTreeItems()
	}
}
