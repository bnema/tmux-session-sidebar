package uity

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

const (
	attentionMarkerSymbol = "\uf0f3"
	heatCurrentColor      = "#6ee7b7"
	heatHotColor          = "#4ade80"
	heatWarmColor         = "#86efac"
	heatCoolColor         = "#94a3b8"
	heatStaleColor        = "#4b5563"
)

type SessionItem struct {
	Name      string
	Current   bool
	Slot      int
	Heat      string
	Attention bool
}

type ProjectItem struct {
	Name string
	Path string
}

type Actions struct {
	SwitchSession       func(string) bool
	CreateProject       func(ProjectItem) bool
	CreateGitProject    func() bool
	CreateAdhoc         func() bool
	RenameSession       func(string) bool
	KillSession         func(string) bool
	ReorderSession      func(string, int) bool
	SetShowNumericItems func(bool) bool
	LoadProjects        func() []ProjectItem
	ReloadSessions      func() []SessionItem
}

type SidebarOptions struct {
	ShowNumericItems bool
	RefreshInterval  time.Duration
}

type SidebarModel struct {
	items           []SessionItem
	cursor          int
	mode            Mode
	filter          string
	showNumeric     bool
	showHelp        bool
	message         string
	projects        []ProjectItem
	projectCursor   int
	projectFilter   string
	pendingKill     string
	refreshInterval time.Duration
	actions         Actions
}

type sidebarStyles struct {
	accent   lipgloss.Style
	dim      lipgloss.Style
	current  lipgloss.Style
	hot      lipgloss.Style
	warm     lipgloss.Style
	cool     lipgloss.Style
	stale    lipgloss.Style
	selected lipgloss.Style
}

type refreshTickMsg struct{}

func NewSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return NewSidebarModelWithOptions(items, actions, SidebarOptions{})
}

func NewSidebarModelWithOptions(items []SessionItem, actions Actions, options SidebarOptions) SidebarModel {
	return SidebarModel{items: items, actions: actions, mode: ModeBrowse, showNumeric: options.ShowNumericItems, refreshInterval: options.RefreshInterval}
}

func (m SidebarModel) Init() tea.Cmd {
	if m.refreshInterval > 0 {
		return scheduleRefresh(m.refreshInterval)
	}
	return nil
}

func (m SidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshTickMsg:
		m.reloadSessions()
		if m.refreshInterval > 0 {
			return m, scheduleRefresh(m.refreshInterval)
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.Keystroke()
		if delta, ok := reorderKeyDelta(msg); ok {
			m.reorderSelected(delta)
			return m, nil
		}
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
		if toggleNumericKey(msg) {
			next := !m.showNumeric
			if m.actions.SetShowNumericItems != nil && !m.actions.SetShowNumericItems(next) {
				return m, nil
			}
			m.showNumeric = next
			return m, nil
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
	if !ok || item.Current || m.actions.SwitchSession == nil {
		return
	}
	if m.actions.SwitchSession(item.Name) {
		m.reloadSessions()
	}
}

func (m *SidebarModel) reloadSessions() {
	if m.actions.ReloadSessions != nil {
		m.items = m.actions.ReloadSessions()
	}
}

func scheduleRefresh(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

func (m *SidebarModel) reorderSelected(delta int) {
	if m.mode != ModeBrowse && m.mode != ModeSearch {
		return
	}
	item, ok := m.selectedSession()
	if !ok || m.actions.ReorderSession == nil || !m.actions.ReorderSession(item.Name, delta) {
		return
	}
	m.reloadSessions()
	m.selectSession(item.Name)
}

func (m *SidebarModel) selectSession(name string) {
	for i, item := range m.visibleItems() {
		if item.Name == name {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
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
		m.mode = ModeBrowse
		m.projectFilter = ""
		m.projectCursor = 0
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
	views := make([]sessions.View, 0, len(m.items))
	byName := make(map[string]SessionItem, len(m.items))
	for _, item := range m.items {
		views = append(views, sessions.View{Name: item.Name, Visible: true})
		byName[item.Name] = item
	}
	visible := sessions.FilterVisible(views, m.showNumeric)
	items := make([]SessionItem, 0, len(visible))
	filter := strings.ToLower(m.filter)
	for _, view := range visible {
		item := byName[view.Name]
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
			styles.dim.Render("M-n project  M-a adhoc  M-H nums"),
			styles.dim.Render("M-J/K reorder  M-r rename"),
			styles.dim.Render("M-x kill  M-? hide"),
		)
	} else {
		lines = append(lines, styles.dim.Render("M-? keys"))
	}
	if m.message != "" {
		lines = append(lines, styles.accent.Render(m.message))
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func sessionHeatColor(item SessionItem) string {
	switch item.Heat {
	case "current":
		return heatCurrentColor
	case "hot":
		return heatHotColor
	case "warm":
		return heatWarmColor
	case "cool":
		return heatCoolColor
	case "stale":
		return heatStaleColor
	default:
		return ""
	}
}

func newSidebarStyles() sidebarStyles {
	return sidebarStyles{
		accent:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc")),
		dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")),
		current:  lipgloss.NewStyle().Foreground(lipgloss.Color(heatCurrentColor)).Bold(true),
		hot:      lipgloss.NewStyle().Foreground(lipgloss.Color(heatHotColor)),
		warm:     lipgloss.NewStyle().Foreground(lipgloss.Color(heatWarmColor)),
		cool:     lipgloss.NewStyle().Foreground(lipgloss.Color(heatCoolColor)),
		stale:    lipgloss.NewStyle().Foreground(lipgloss.Color(heatStaleColor)),
		selected: lipgloss.NewStyle().Background(lipgloss.Color("#1f2937")).Foreground(lipgloss.Color("#ffffff")).Bold(true),
	}
}

func sessionRowStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	if item.Current {
		return styles.current
	}
	switch item.Heat {
	case "hot":
		return styles.hot
	case "warm":
		return styles.warm
	case "cool":
		return styles.cool
	case "stale":
		return styles.stale
	default:
		return lipgloss.NewStyle()
	}
}

func sessionMarker(item SessionItem) string {
	if item.Current {
		return "*"
	}
	if item.Attention {
		return attentionMarkerSymbol
	}
	return " "
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
		badge := "    "
		if item.Slot > 0 {
			badge = fmt.Sprintf("[%s] ", slotLabel(item.Slot))
		}
		style := sessionRowStyle(styles, item)
		row := style.Render(fmt.Sprintf("%s %s%s", sessionMarker(item), badge, item.Name))
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

func reorderKeyDelta(msg tea.KeyPressMsg) (int, bool) {
	key := msg.Key()
	if !key.Mod.Contains(tea.ModAlt) {
		return 0, false
	}
	switch key.Code {
	case tea.KeyDown:
		return 1, true
	case tea.KeyUp:
		return -1, true
	case 'j', 'J':
		return 1, true
	case 'k', 'K':
		return -1, true
	}
	switch key.Text {
	case "j", "J":
		return 1, true
	case "k", "K":
		return -1, true
	default:
		return 0, false
	}
}

func toggleNumericKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if !key.Mod.Contains(tea.ModAlt) {
		return false
	}
	return key.Text == "h" || key.Text == "H" || key.Code == 'h' || key.Code == 'H'
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
