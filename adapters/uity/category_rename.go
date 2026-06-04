package uity

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *SidebarModel) handleRenameCategoryKey(msg tea.KeyPressMsg) {
	switch msg.Keystroke() {
	case "enter":
		m.confirmRenameCategory()
	case "esc":
		m.clearRenameCategory()
	case "backspace":
		if m.renameCategoryInput != "" {
			m.renameCategoryInput = trimLastRune(m.renameCategoryInput)
		}
	default:
		if key, ok := printableKey(msg); ok {
			m.renameCategoryInput += key
		}
	}
}

func (m *SidebarModel) confirmRenameCategory() {
	id := m.renameCategoryID
	name := strings.TrimSpace(m.renameCategoryInput)
	if id == "" || name == "" || m.actions.RenameCategory == nil || !m.actions.RenameCategory(id, name) {
		return
	}
	m.clearRenameCategory()
	m.reloadTreeItems()
	m.selectTreeItem(id)
}

func (m *SidebarModel) clearRenameCategory() {
	m.mode = ModeBrowse
	m.renameCategoryID = ""
	m.renameCategoryInput = ""
}
