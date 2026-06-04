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
	createCategory       = "category"
	createSeparator      = "separator"
	createSpacer         = "spacer"
)

func (m *SidebarModel) openCreateMenu() {
	m.openMenu(ModeCreate, []menuItem{
		{Label: "Git repo", Value: createSessionGit},
		{Label: "Current dir", Value: createSessionCurrent},
		{Label: "Named…", Value: createSessionNamed},
		{Label: "Project…", Value: createSessionProject},
		{Label: "Category", Value: createCategory},
		{Label: "Separator", Value: createSeparator},
		{Label: "Empty space", Value: createSpacer},
	}, menuSpec{Title: "create", Footer: "", EmptyLabel: "no items", Height: 10, Choose: chooseCreate})
}

func (m *SidebarModel) startCreateNamed() {
	m.menu = menuState{}
	m.mode = ModeCreateNamed
	m.createNamedInput = ""
}

func chooseCreate(m *SidebarModel, choice menuItem) {
	sessionCreated := false
	layoutChanged := false
	switch choice.Value {
	case createSessionGit:
		if m.actions.CreateGitProject != nil {
			sessionCreated = m.actions.CreateGitProject()
		}
	case createSessionCurrent:
		if m.actions.CreateAdhoc != nil {
			sessionCreated = m.actions.CreateAdhoc()
		}
	case createSessionNamed:
		m.startCreateNamed()
		return
	case createSessionProject:
		m.openProjectMenu()
		return
	case createCategory:
		if m.actions.CreateCategory != nil {
			layoutChanged = m.actions.CreateCategory("New category")
		}
	case createSeparator:
		if m.actions.CreateSeparator != nil {
			layoutChanged = m.actions.CreateSeparator()
		}
	case createSpacer:
		if m.actions.CreateSpacer != nil {
			layoutChanged = m.actions.CreateSpacer()
		}
	}
	if sessionCreated {
		m.closeMenu()
		m.reloadSessionsSelectingCurrent()
	}
	if layoutChanged {
		m.closeMenu()
		m.reloadTreeItems()
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
