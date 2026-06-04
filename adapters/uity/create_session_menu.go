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
		{Label: "sessions", Header: true},
		{Label: "new named session", Value: createSessionNamed},
		{Label: "from git repo", Value: createSessionGit},
		{Label: "from pwd", Value: createSessionCurrent},
		{Label: "from project picker", Value: createSessionProject},
		{Label: "layout", Header: true},
		{Label: "category", Value: createCategory},
		{Label: "separator", Value: createSeparator},
		{Label: "empty space", Value: createSpacer},
	}, menuSpec{Title: "CREATE MENU", Footer: "↵ create   esc close", EmptyLabel: "no items", Height: 13, Choose: chooseCreate})
}

func (m *SidebarModel) startCreateNamed() {
	m.menu = menuState{}
	m.mode = ModeCreateNamed
	m.createNamedInput = ""
	m.createTargetCategoryID = m.selectedCategoryID()
}

func (m *SidebarModel) startCreateCategory() {
	m.menu = menuState{}
	m.mode = ModeCreateCategory
	m.createCategoryInput = ""
}

func chooseCreate(m *SidebarModel, choice menuItem) {
	sessionCreated := false
	layoutChanged := false
	switch choice.Value {
	case createSessionGit:
		if m.actions.CreateGitProject != nil {
			sessionCreated = m.actions.CreateGitProject(m.selectedCategoryID())
		}
	case createSessionCurrent:
		if m.actions.CreateAdhoc != nil {
			sessionCreated = m.actions.CreateAdhoc(m.selectedCategoryID())
		}
	case createSessionNamed:
		m.startCreateNamed()
		return
	case createSessionProject:
		m.createTargetCategoryID = m.selectedCategoryID()
		m.openProjectMenu()
		return
	case createCategory:
		m.startCreateCategory()
		return
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
	if name == "" || m.actions.CreateNamedSession == nil || !m.actions.CreateNamedSession(name, m.createTargetCategoryID) {
		return
	}
	m.clearCreateNamed()
	m.reloadSessionsSelectingCurrent()
}

func (m *SidebarModel) clearCreateNamed() {
	m.mode = ModeBrowse
	m.createNamedInput = ""
	m.createTargetCategoryID = ""
}

func (m *SidebarModel) handleCreateCategoryKey(msg tea.KeyPressMsg) {
	switch msg.Keystroke() {
	case "enter":
		m.confirmCreateCategory()
	case "esc":
		m.clearCreateCategory()
	case "backspace":
		if m.createCategoryInput != "" {
			m.createCategoryInput = trimLastRune(m.createCategoryInput)
		}
	default:
		if key, ok := printableKey(msg); ok {
			m.createCategoryInput += key
		}
	}
}

func (m *SidebarModel) confirmCreateCategory() {
	name := strings.TrimSpace(m.createCategoryInput)
	if name == "" || m.actions.CreateCategory == nil || !m.actions.CreateCategory(name) {
		return
	}
	m.clearCreateCategory()
	m.reloadTreeItems()
}

func (m *SidebarModel) clearCreateCategory() {
	m.mode = ModeBrowse
	m.createCategoryInput = ""
}
