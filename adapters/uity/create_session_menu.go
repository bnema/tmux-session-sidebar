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

func (m *SidebarModel) openCreateSessionMenu() {
	m.openMenu(ModeCreateSession, []menuItem{
		{Label: "Git repo", Value: createSessionGit},
		{Label: "Current dir", Value: createSessionCurrent},
		{Label: "Named…", Value: createSessionNamed},
		{Label: "Project…", Value: createSessionProject},
	}, menuSpec{Title: "create session", Footer: "esc cancel  ↵ choose", EmptyLabel: "no items", Height: 8, Choose: chooseCreateSession})
}

func chooseCreateSession(m *SidebarModel, choice menuItem) {
	success := false
	switch choice.Value {
	case createSessionGit:
		if m.actions.CreateGitProject != nil {
			success = m.actions.CreateGitProject()
		}
	case createSessionCurrent:
		if m.actions.CreateAdhoc != nil {
			success = m.actions.CreateAdhoc()
		}
	case createSessionNamed:
		m.menu = menuState{}
		m.mode = ModeCreateNamed
		m.createNamedInput = ""
		return
	case createSessionProject:
		m.openProjectMenu()
		return
	}
	if success {
		m.closeMenu()
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
