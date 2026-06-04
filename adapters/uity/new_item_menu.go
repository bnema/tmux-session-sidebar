package uity

const (
	newItemCategory  = "category"
	newItemSpacer    = "spacer"
	newItemSeparator = "separator"
)

func (m *SidebarModel) openNewItemMenu() {
	m.openMenu(ModeNewItem, []menuItem{
		{Label: "New category", Value: newItemCategory},
		{Label: "New spacer", Value: newItemSpacer},
		{Label: "New separator", Value: newItemSeparator},
	}, menuSpec{Title: "new layout item", Footer: "esc cancel  ↵ create", EmptyLabel: "no items", Height: 7, Choose: chooseNewItem})
}

func chooseNewItem(m *SidebarModel, choice menuItem) {
	ok := false
	switch choice.Value {
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
		m.closeMenu()
		m.reloadTreeItems()
	}
}
