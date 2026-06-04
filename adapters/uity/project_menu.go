package uity

func (m *SidebarModel) openProjectMenu() {
	projects := []ProjectItem{}
	if m.actions.LoadProjects != nil {
		projects = m.actions.LoadProjects()
	}
	m.openMenu(ModeProject, projectMenuItems(projects), menuSpec{Title: "projects", Footer: "esc cancel  ↵ create", EmptyLabel: "no projects", Filterable: true, Height: 8, Choose: chooseProject})
}

func chooseProject(m *SidebarModel, item menuItem) {
	categoryID := m.createTargetCategoryID
	if categoryID == "" {
		categoryID = m.selectedCategoryID()
	}
	if m.actions.CreateProject != nil && m.actions.CreateProject(item.Project, categoryID) {
		m.createTargetCategoryID = ""
		m.closeMenu()
		m.reloadSessionsSelectingCurrent()
	}
}
