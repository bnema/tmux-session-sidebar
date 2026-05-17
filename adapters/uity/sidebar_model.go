package uity

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

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
	SwitchSession    func(string)
	CreateProject    func(ProjectItem)
	CreateGitProject func()
	CreateAdhoc      func()
	RenameSession    func(string)
	KillSession      func(string)
	LoadProjects     func() []ProjectItem
}

type SidebarModel struct {
	items         []SessionItem
	cursor        int
	mode          Mode
	filter        string
	showNumeric   bool
	message       string
	projects      []ProjectItem
	projectCursor int
	projectFilter string
	actions       Actions
}

func NewSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return SidebarModel{items: items, actions: actions, mode: ModeBrowse}
}

func (m SidebarModel) Init() tea.Cmd { return nil }

func (m SidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
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
			if m.actions.CreateGitProject != nil {
				m.actions.CreateGitProject()
			}
			return m, nil
		case "alt+a":
			if m.actions.CreateAdhoc != nil {
				m.actions.CreateAdhoc()
			}
			return m, nil
		case "alt+r":
			if item, ok := m.selectedSession(); ok && m.actions.RenameSession != nil {
				m.actions.RenameSession(item.Name)
			}
			return m, nil
		case "alt+x":
			if item, ok := m.selectedSession(); ok && m.actions.KillSession != nil {
				m.actions.KillSession(item.Name)
			}
			return m, nil
		case "alt+h":
			m.showNumeric = !m.showNumeric
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
			if m.mode == ModeSearch {
				m.filter += msg.String()
			}
			if m.mode == ModeProject {
				m.projectFilter += msg.String()
			}
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

func (m SidebarModel) switchSelected() {
	item, ok := m.selectedSession()
	if ok && m.actions.SwitchSession != nil {
		m.actions.SwitchSession(item.Name)
	}
}

func (m SidebarModel) createSelectedProject() {
	visible := m.visibleProjects()
	if len(visible) == 0 || m.projectCursor >= len(visible) {
		return
	}
	if m.actions.CreateProject != nil {
		m.actions.CreateProject(visible[m.projectCursor])
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

func (m SidebarModel) View() string {
	accent := lipgloss.Color("#7dd3fc")
	dim := lipgloss.Color("#6b7280")
	green := lipgloss.Color("#86efac")
	warm := lipgloss.Color("#a7f3d0")
	selected := lipgloss.NewStyle().Background(lipgloss.Color("#1f2937")).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("sessions")
	modeText := "browse"
	if m.mode == ModeSearch {
		modeText = "filter:" + m.filter
	}
	if m.mode == ModeProject {
		modeText = "projects:" + m.projectFilter
	}
	meta := lipgloss.NewStyle().Foreground(dim).Render(modeText)
	currentStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	warmStyle := lipgloss.NewStyle().Foreground(warm)
	dimStyle := lipgloss.NewStyle().Foreground(dim)
	lines := []string{header + " " + meta, ""}
	if m.mode == ModeProject {
		lines = append(lines, m.renderProjects(selected, dimStyle)...)
	} else {
		lines = append(lines, m.renderSessions(selected, currentStyle, warmStyle, dimStyle)...)
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(dim).Render("↵ choose  / filter  esc back"), lipgloss.NewStyle().Foreground(dim).Render("M-n project  M-a adhoc  M-h nums"), lipgloss.NewStyle().Foreground(dim).Render("M-r rename   M-x kill"))
	if m.message != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(accent).Render(m.message))
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m SidebarModel) renderSessions(selected lipgloss.Style, currentStyle lipgloss.Style, warmStyle lipgloss.Style, dimStyle lipgloss.Style) []string {
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
			style = currentStyle
		} else if item.Heat == "hot" || item.Heat == "warm" {
			style = warmStyle
		}
		row := style.Render(fmt.Sprintf("%s %s%s", marker, badge, item.Name))
		if i == m.cursor {
			row = selected.Render(row)
		}
		lines = append(lines, row)
	}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("no sessions"))
	}
	return lines
}

func (m SidebarModel) renderProjects(selected lipgloss.Style, dimStyle lipgloss.Style) []string {
	visible := m.visibleProjects()
	lines := make([]string, 0, len(visible)+1)
	for i, project := range visible {
		row := fmt.Sprintf("  %-18s", project.Name)
		if i == m.projectCursor {
			row = selected.Render(row)
		}
		lines = append(lines, row)
	}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("no projects"))
	}
	return lines
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
