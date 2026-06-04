package uity

import "strings"

type menuItem struct {
	Label       string
	Description string
	Value       string
	Project     ProjectItem
	Header      bool
}

type menuSpec struct {
	Title      string
	Footer     string
	EmptyLabel string
	Filterable bool
	Height     int
	Choose     func(*SidebarModel, menuItem)
}

type menuState struct {
	Items  []menuItem
	Cursor int
	Filter string
	Spec   menuSpec
}

func (m SidebarModel) menuActive() bool {
	return m.mode == ModeProject || m.mode == ModeCreate
}

func (m *SidebarModel) openMenu(mode Mode, items []menuItem, spec menuSpec) {
	m.mode = mode
	m.menu = menuState{Items: items, Spec: spec}
	m.clampMenuCursor()
}

func (m *SidebarModel) closeMenu() {
	m.menu = menuState{}
	m.mode = ModeBrowse
}

func (m SidebarModel) visibleMenuItems() []menuItem {
	items := make([]menuItem, 0, len(m.menu.Items))
	filter := strings.ToLower(strings.TrimSpace(m.menu.Filter))
	for _, item := range m.menu.Items {
		if m.menu.Spec.Filterable && filter != "" && !strings.Contains(strings.ToLower(item.Label), filter) {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (m *SidebarModel) moveMenu(delta int) {
	visible := m.visibleMenuItems()
	if len(visible) == 0 || !hasSelectableMenuItem(visible) {
		m.menu.Cursor = 0
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for range len(visible) {
		m.menu.Cursor = (m.menu.Cursor + step + len(visible)) % len(visible)
		if !visible[m.menu.Cursor].Header {
			return
		}
	}
}

func (m *SidebarModel) chooseMenuItem() {
	visible := m.visibleMenuItems()
	if len(visible) == 0 || m.menu.Cursor >= len(visible) || visible[m.menu.Cursor].Header || m.menu.Spec.Choose == nil {
		return
	}
	m.menu.Spec.Choose(m, visible[m.menu.Cursor])
}

func (m *SidebarModel) backspaceMenuFilter() {
	if m.menu.Spec.Filterable && m.menu.Filter != "" {
		m.menu.Filter = trimLastRune(m.menu.Filter)
		m.clampMenuCursor()
	}
}

func (m *SidebarModel) appendMenuFilter(key string) {
	if m.menu.Spec.Filterable {
		m.menu.Filter += key
		m.clampMenuCursor()
	}
}

func (m *SidebarModel) clampMenuCursor() {
	visible := m.visibleMenuItems()
	if len(visible) == 0 {
		m.menu.Cursor = 0
		return
	}
	if m.menu.Cursor >= len(visible) {
		m.menu.Cursor = len(visible) - 1
	}
	if !visible[m.menu.Cursor].Header {
		return
	}
	for i, item := range visible {
		if !item.Header {
			m.menu.Cursor = i
			return
		}
	}
	m.menu.Cursor = 0
}

func (m SidebarModel) renderMenuRows(styles sidebarStyles) string {
	visible := m.visibleMenuItems()
	if len(visible) == 0 {
		empty := m.menu.Spec.EmptyLabel
		if empty == "" {
			empty = "no items"
		}
		return styles.dim.Render(empty)
	}
	lines := make([]string, 0, len(visible)+1)
	for i, item := range visible {
		if item.Header {
			lines = append(lines, styles.accent.Render(strings.ToLower(item.Label)))
			continue
		}
		selected := i == m.menu.Cursor
		prefix := "  "
		if selected {
			prefix = "> "
		}
		line := prefix + item.Label
		if selected {
			line = styles.selected.Render(line)
		}
		lines = append(lines, line)
		if selected && strings.TrimSpace(item.Description) != "" {
			lines = append(lines, styles.dim.Render("  "+item.Description))
		}
	}
	return strings.Join(lines, "\n")
}

func hasSelectableMenuItem(items []menuItem) bool {
	for _, item := range items {
		if !item.Header {
			return true
		}
	}
	return false
}

func projectMenuItems(projects []ProjectItem) []menuItem {
	items := make([]menuItem, 0, len(projects))
	for _, project := range projects {
		items = append(items, menuItem{Label: project.Name, Project: project})
	}
	return items
}
