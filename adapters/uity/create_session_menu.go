package uity

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	createSessionGit     = "git"
	createSessionCurrent = "current"
	createSessionNamed   = "named"
	createSessionProject = "project"
)

func createSessionMenuItems() []ProjectItem {
	return []ProjectItem{
		{Name: "Git repo", Path: createSessionGit},
		{Name: "Current dir", Path: createSessionCurrent},
		{Name: "Named…", Path: createSessionNamed},
		{Name: "Project…", Path: createSessionProject},
	}
}

func (m *SidebarModel) createSelectedSessionChoice() {
	visible := m.visibleProjects()
	if len(visible) == 0 || m.projectCursor >= len(visible) {
		return
	}
	choice := visible[m.projectCursor]
	success := false
	switch choice.Path {
	case createSessionGit:
		if m.actions.CreateGitProject != nil {
			success = m.actions.CreateGitProject()
		}
	case createSessionCurrent:
		if m.actions.CreateAdhoc != nil {
			success = m.actions.CreateAdhoc()
		}
	case createSessionNamed:
		m.mode = ModeCreateNamed
		m.createNamedInput = ""
		return
	case createSessionProject:
		if m.actions.LoadProjects != nil {
			m.projects = m.actions.LoadProjects()
			m.mode = ModeProject
			m.projectCursor = 0
			m.projectFilter = ""
		}
		return
	}
	if success {
		m.mode = ModeBrowse
		m.projectCursor = 0
		m.projectFilter = ""
		m.reloadSessionsSelectingCurrent()
	}
}

func (m *SidebarModel) handleCreateNamedKey(msg tea.KeyPressMsg) {
	switch msg.Keystroke() {
	case "enter":
		m.confirmCreateNamed()
	case "esc":
		m.clearCreateNamed()
	case "backspace":
		if m.createNamedInput != "" {
			m.createNamedInput = trimLastRune(m.createNamedInput)
		}
	default:
		if key, ok := printableKey(msg); ok {
			m.createNamedInput += key
		}
	}
}

func (m *SidebarModel) confirmCreateNamed() {
	name := strings.TrimSpace(m.createNamedInput)
	if name == "" || m.actions.CreateNamedSession == nil || !m.actions.CreateNamedSession(name) {
		return
	}
	m.clearCreateNamed()
	m.reloadSessionsSelectingCurrent()
}

func (m *SidebarModel) clearCreateNamed() {
	m.mode = ModeBrowse
	m.createNamedInput = ""
}
