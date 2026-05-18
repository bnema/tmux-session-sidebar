package uity

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

type SessionItem struct {
	Name    string
	Current bool
	Slot    int
	Heat    string
}

type ProjectItem struct {
	Name string
	Path string
}

type Actions struct {
	SwitchSession    func(string) bool
	CreateProject    func(ProjectItem) bool
	CreateGitProject func() bool
	CreateAdhoc      func() bool
	RenameSession    func(string) bool
	KillSession      func(string) bool
	LoadProjects     func() []ProjectItem
	ReloadSessions   func() []SessionItem
}

type SidebarModel struct {
	items         []SessionItem
	cursor        int
	mode          Mode
	filter        string
	showNumeric   bool
	showHelp      bool
	message       string
	projects      []ProjectItem
	projectCursor int
	projectFilter string
	pendingKill   string
	actions       Actions
}

type sidebarStyles struct {
	accent   lipgloss.Style
	dim      lipgloss.Style
	current  lipgloss.Style
	warm     lipgloss.Style
	selected lipgloss.Style
}

func NewSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return SidebarModel{items: items, actions: actions, mode: ModeBrowse}
}

func (m SidebarModel) Init() tea.Cmd { return nil }

func (m SidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.Keystroke()
		if m.mode == ModeConfirmKill {
			if isKillConfirmYes(msg) {
				m.confirmKill()
				return m, nil
			}
			if isKillConfirmCancel(msg) {
				m.clearKillConfirmation()
				return m, nil
			}
			if key != "ctrl+c" {
				return m, nil
			}
		}
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.mode == ModeSearch {
				m.mode = ModeBrowse
				m.filter = ""
				return m, nil
			}
			if m.mode == ModeProject {
				m.mode = ModeBrowse
				m.projectFilter = ""
				return m, nil
			}
			if m.mode == ModeConfirmKill {
				m.clearKillConfirmation()
				return m, nil
			}
			return m, tea.Quit
		case "/":
			if m.mode == ModeBrowse {
				m.mode = ModeSearch
			}
			return m, nil
		case "enter":
			if m.mode == ModeSearch {
				m.mode = ModeBrowse
				return m, nil
			}
			if m.mode == ModeProject {
				m.createSelectedProject()
				return m, nil
			}
			if m.mode == ModeConfirmKill {
				m.clearKillConfirmation()
				return m, nil
			}
			m.switchSelected()
			return m, nil
		case "j", "down":
			if m.mode == ModeProject {
				m.moveProject(1)
			} else {
				m.move(1)
			}
			return m, nil
		case "k", "up":
			if m.mode == ModeProject {
				m.moveProject(-1)
			} else {
				m.move(-1)
			}
			return m, nil
		case "alt+n":
			if m.actions.LoadProjects != nil {
				m.projects = m.actions.LoadProjects()
			}
			m.mode = ModeProject
			m.projectCursor = 0
			m.projectFilter = ""
			return m, nil
		case "alt+g":
			if m.actions.CreateGitProject != nil && m.actions.CreateGitProject() {
				m.reloadSessions()
			}
			return m, nil
		case "alt+a":
			if m.actions.CreateAdhoc != nil && m.actions.CreateAdhoc() {
				m.reloadSessions()
			}
			return m, nil
		case "alt+r":
			if item, ok := m.selectedSession(); ok && m.actions.RenameSession != nil && m.actions.RenameSession(item.Name) {
				m.reloadSessions()
			}
			return m, nil
		case "alt+x":
			if item, ok := m.selectedSession(); ok && m.actions.KillSession != nil {
				m.mode = ModeConfirmKill
				m.pendingKill = item.Name
				m.message = "Kill " + item.Name + "? y/N"
			}
			return m, nil
		case "y", "n":
			m.appendPrintable(msg)
		case "f5":
			m.reloadSessions()
			return m, nil
		case "alt+h":
			m.showNumeric = !m.showNumeric
			return m, nil
		case "alt+?":
			m.showHelp = !m.showHelp
			return m, nil
		case "backspace":
			if m.mode == ModeSearch && m.filter != "" {
				m.filter = trimLastRune(m.filter)
			}
			if m.mode == ModeProject && m.projectFilter != "" {
				m.projectFilter = trimLastRune(m.projectFilter)
			}
			return m, nil
		default:
			m.appendPrintable(msg)
		}
	}
	return m, nil
}

func (m *SidebarModel) move(delta int) {
	visible := m.visibleItems()
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = (m.cursor + delta + len(visible)) % len(visible)
}

func (m *SidebarModel) moveProject(delta int) {
	visible := m.visibleProjects()
	if len(visible) == 0 {
		m.projectCursor = 0
		return
	}
	m.projectCursor = (m.projectCursor + delta + len(visible)) % len(visible)
}

func (m SidebarModel) selectedSession() (SessionItem, bool) {
	visible := m.visibleItems()
	if len(visible) == 0 || m.cursor >= len(visible) {
		return SessionItem{}, false
	}
	return visible[m.cursor], true
}

func (m *SidebarModel) switchSelected() {
	item, ok := m.selectedSession()
	if ok && m.actions.SwitchSession != nil && m.actions.SwitchSession(item.Name) {
		m.reloadSessions()
	}
}

func (m *SidebarModel) reloadSessions() {
	if m.actions.ReloadSessions != nil {
		m.items = m.actions.ReloadSessions()
	}
}

func (m *SidebarModel) confirmKill() {
	name := m.pendingKill
	m.clearKillConfirmation()
	if name != "" && m.actions.KillSession != nil && m.actions.KillSession(name) {
		m.reloadSessions()
	}
}

func (m *SidebarModel) clearKillConfirmation() {
	m.mode = ModeBrowse
	m.pendingKill = ""
	m.message = ""
}

func (m *SidebarModel) createSelectedProject() {
	visible := m.visibleProjects()
	if len(visible) == 0 || m.projectCursor >= len(visible) {
		return
	}
	if m.actions.CreateProject != nil && m.actions.CreateProject(visible[m.projectCursor]) {
		m.reloadSessions()
	}
}

func (m *SidebarModel) appendPrintable(msg tea.KeyPressMsg) {
	if key, ok := printableKey(msg); ok {
		if m.mode == ModeSearch {
			m.filter += key
		}
		if m.mode == ModeProject {
			m.projectFilter += key
		}
	}
}

func (m SidebarModel) visibleItems() []SessionItem {
	items := make([]SessionItem, 0, len(m.items))
	filter := strings.ToLower(m.filter)
	for _, item := range m.items {
		if strings.HasPrefix(item.Name, "__") {
			continue
		}
		if !m.showNumeric && sessions.IsNumericName(item.Name) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(item.Name), filter) {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (m SidebarModel) visibleProjects() []ProjectItem {
	projects := make([]ProjectItem, 0, len(m.projects))
	filter := strings.ToLower(m.projectFilter)
	for _, project := range m.projects {
		if filter != "" && !strings.Contains(strings.ToLower(project.Name), filter) {
			continue
		}
		projects = append(projects, project)
	}
	return projects
}

func (m SidebarModel) View() tea.View {
	return tea.NewView(m.Render())
}

func (m SidebarModel) Render() string {
	styles := newSidebarStyles()
	lines := []string{""}
	if m.mode == ModeProject {
		lines = append(lines, m.renderProjects(styles)...)
	} else {
		lines = append(lines, m.renderSessions(styles)...)
	}
	if status := m.statusLine(); status != "" {
		lines = append(lines, "", styles.accent.Render(status))
	}
	lines = append(lines, "")
	if m.showHelp {
		lines = append(lines,
			styles.dim.Render("↵ choose  / filter  esc back  M-b toggle"),
			styles.dim.Render("M-n project  M-a adhoc  M-h nums"),
			styles.dim.Render("M-r rename   M-x kill  M-? hide"),
		)
	} else {
		lines = append(lines, styles.dim.Render("M-? keys"))
	}
	if m.message != "" {
		lines = append(lines, styles.accent.Render(m.message))
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func newSidebarStyles() sidebarStyles {
	return sidebarStyles{
		accent:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc")),
		dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")),
		current: lipgloss.NewStyle().Foreground(lipgloss.Color("#86efac")).Bold(true),
		warm:    lipgloss.NewStyle().Foreground(lipgloss.Color("#a7f3d0")),
		selected: lipgloss.NewStyle().Background(lipgloss.Color("#1f2937")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true),
	}
}

func (m SidebarModel) statusLine() string {
	switch m.mode {
	case ModeSearch:
		return "filter: " + m.filter
	case ModeProject:
		return "projects: " + m.projectFilter
	case ModeConfirmKill:
		return "confirm kill"
	default:
		return ""
	}
}

func (m SidebarModel) renderSessions(styles sidebarStyles) []string {
	visible := m.visibleItems()
	lines := make([]string, 0, len(visible)+1)
	for i, item := range visible {
		marker := " "
		if item.Current {
			marker = "*"
		}
		badge := "    "
		if item.Slot > 0 {
			badge = fmt.Sprintf("[%s] ", slotLabel(item.Slot))
		}
		style := lipgloss.NewStyle()
		if item.Current {
			style = styles.current
		} else if item.Heat == "hot" || item.Heat == "warm" {
			style = styles.warm
		}
		row := style.Render(fmt.Sprintf("%s %s%s", marker, badge, item.Name))
		if i == m.cursor {
			row = styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	if len(visible) == 0 {
		lines = append(lines, styles.dim.Render("no sessions"))
	}
	return lines
}

func (m SidebarModel) renderProjects(styles sidebarStyles) []string {
	visible := m.visibleProjects()
	lines := make([]string, 0, len(visible)+1)
	for i, project := range visible {
		row := fmt.Sprintf("  %-18s", project.Name)
		if i == m.projectCursor {
			row = styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	if len(visible) == 0 {
		lines = append(lines, styles.dim.Render("no projects"))
	}
	return lines
}

func isKillConfirmYes(msg tea.KeyPressMsg) bool {
	return msg.Key().Text == "y" || msg.Key().Text == "Y"
}

func isKillConfirmCancel(msg tea.KeyPressMsg) bool {
	key := msg.Keystroke()
	return msg.Key().Text == "n" || msg.Key().Text == "N" || key == "enter" || key == "esc"
}

func printableKey(msg tea.KeyPressMsg) (string, bool) {
	value := msg.Key().Text
	runes := []rune(value)
	if len(runes) != 1 || !unicode.IsPrint(runes[0]) {
		return "", false
	}
	return string(runes[0]), true
}

func trimLastRune(value string) string {
	r := []rune(value)
	if len(r) == 0 {
		return value
	}
	return string(r[:len(r)-1])
}
func slotLabel(slot int) string {
	if slot == 10 {
		return "0"
	}
	return fmt.Sprintf("%d", slot)
}
