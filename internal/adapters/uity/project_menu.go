package uity

func (m *SidebarModel) openProjectMenu() {
	projects := []ProjectItem{}
	if m.actions.LoadProjects != nil {
		projects = m.actions.LoadProjects()
	}
	m.openMenu(ModeProject, projectMenuItems(projects), menuSpec{Title: "projects", Footer: "type to filter  ↑/↓ or alt+j/k navigate  ↵ create  esc cancel", EmptyLabel: "No projects found", Filterable: true, Height: 8, Choose: chooseProject})
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
