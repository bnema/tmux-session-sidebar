package uity

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

const attentionMarkerSymbol = "\uf0f3" // Nerd Font bell glyph (U+F0F3 / nf-fa-bell).
const currentMarkerSymbol = "\uf444"   // Nerd Font dot-fill glyph (U+F444 / nf-oct-dot_fill).
const pinnedMarkerSymbol = "\uf08d"    // Nerd Font thumb-tack glyph (U+F08D / nf-fa-thumb_tack).
const spaceKeySymbol = "\U000F1050"    // Nerd Font keyboard-space glyph (U+F1050 / nf-md-keyboard_space).

type SessionItem struct {
	Name          string
	Current       bool
	Slot          int
	Heat          string
	HeatIntensity float64
	Attention     bool
	Pinned        bool
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
	TogglePinnedSession func(string) bool
	ReorderSession      func(string, int) bool
	SetShowNumericItems func(bool) bool
	LoadProjects        func() []ProjectItem
	ReloadSessions      func() []SessionItem
}

type SidebarOptions struct {
	ShowNumericItems bool
	Version          string
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
	version       string
	height        int
}

type sidebarStyles struct {
	accent       lipgloss.Style
	dim          lipgloss.Style
	active       lipgloss.Style
	stale        lipgloss.Style
	selected     lipgloss.Style
	pinned       lipgloss.Style
	versionBadge lipgloss.Style
}

func NewSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return NewSidebarModelWithOptions(items, actions, SidebarOptions{})
}

func NewSidebarModelWithOptions(items []SessionItem, actions Actions, options SidebarOptions) SidebarModel {
	return SidebarModel{items: items, actions: actions, mode: ModeBrowse, showNumeric: options.ShowNumericItems, version: options.Version}
}

func (m SidebarModel) Init() tea.Cmd {
	return nil
}

func (m SidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
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
		if pinnedToggleKey(msg) && m.mode == ModeBrowse {
			m.togglePinnedSelected()
			return m, nil
		}
		if slot, ok := numericSlotKey(msg); ok && m.mode == ModeBrowse {
			m.switchSlot(slot)
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
				m.reloadSessionsSelectingCurrent()
			}
			return m, nil
		case "alt+a":
			if m.actions.CreateAdhoc != nil && m.actions.CreateAdhoc() {
				m.reloadSessionsSelectingCurrent()
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
	if !ok {
		return
	}
	m.switchItem(item)
}

func (m *SidebarModel) switchSlot(slot int) {
	for _, item := range m.visibleItems() {
		if item.Slot == slot {
			m.switchItem(item)
			return
		}
	}
}

func (m *SidebarModel) switchItem(item SessionItem) {
	if item.Current || m.actions.SwitchSession == nil {
		return
	}
	if m.actions.SwitchSession(item.Name) {
		m.reloadSessions()
		m.selectSession(item.Name)
	}
}

func (m SidebarModel) currentSessionName() string {
	for _, item := range m.items {
		if item.Current {
			return item.Name
		}
	}
	return ""
}

func (m *SidebarModel) reloadSessions() {
	m.reloadSessionsWithSelection(true)
}

func (m *SidebarModel) reloadSessionsSelectingCurrent() {
	m.reloadSessionsWithSelection(false)
}

func (m *SidebarModel) reloadSessionsWithSelection(preservePreviousCurrent bool) {
	if m.actions.ReloadSessions == nil {
		return
	}
	// Preserve selection on the previously-current session when a refresh observes
	// a session switch, so the sidebar is ready to toggle back to where the user came from.
	previous := m.currentSessionName()
	m.items = m.actions.ReloadSessions()
	current := m.currentSessionName()
	if current == "" {
		return
	}
	if preservePreviousCurrent && previous != "" && previous != current {
		m.selectSession(previous)
		return
	}
	if !preservePreviousCurrent {
		m.selectSession(current)
	}
}

func (m *SidebarModel) togglePinnedSelected() {
	item, ok := m.selectedSession()
	if !ok || m.actions.TogglePinnedSession == nil || !m.actions.TogglePinnedSession(item.Name) {
		return
	}
	m.reloadSessions()
	m.selectSession(item.Name)
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
		m.reloadSessionsSelectingCurrent()
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
	if m.message != "" {
		lines = append(lines, "", styles.accent.Render(m.message))
	}
	lines = StatusBar{Lines: m.statusBarLines(styles), Height: m.height}.RenderBelow(padSidebarContentLines(lines))
	return strings.Join(lines, "\n")
}

func padSidebarContentLines(lines []string) []string {
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = " " + line + " "
	}
	return padded
}

func (m SidebarModel) statusBarLines(styles sidebarStyles) []string {
	if m.showHelp {
		return []string{
			styles.dim.Render("↵ choose  " + spaceKeySymbol + " pin  / filter  esc back"),
			styles.dim.Render("M-n project  M-a adhoc  M-H nums"),
			styles.dim.Render("M-J/K reorder  M-r rename"),
			styles.dim.Render("M-x kill  M-? hide"),
		}
	}
	return []string{m.collapsedHelpLine(styles)}
}

func (m SidebarModel) collapsedHelpLine(styles sidebarStyles) string {
	version := displayVersion(m.version)
	if version == "" {
		return styles.dim.Render("M-? keys")
	}
	return styles.versionBadge.Render(" "+version+" ") + styles.dim.Render(" M-? keys")
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func newSidebarStyles() sidebarStyles {
	return sidebarStyles{
		accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc")),
		dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")),
		active:       lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true),
		stale:        lipgloss.NewStyle().Foreground(lipgloss.Color(inactiveSessionRGB.Hex())),
		selected:     lipgloss.NewStyle().Background(lipgloss.Color("#065f46")).Foreground(lipgloss.Color("#ecfdf5")).Bold(true),
		pinned:       lipgloss.NewStyle().Foreground(lipgloss.Color("#facc15")).Bold(true),
		versionBadge: lipgloss.NewStyle().Background(lipgloss.Color("#334155")).Foreground(lipgloss.Color("#e0f2fe")).Bold(true),
	}
}

func sessionRowStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	if item.Pinned {
		return styles.pinned
	}
	if item.Current {
		return styles.active
	}
	if item.Heat == "" || item.Heat == string(heat.BucketStale) {
		return styles.stale
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(heatColor(item.HeatIntensity)))
}

func sessionMarkerStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	if item.Attention || item.Current {
		return styles.active
	}
	if item.Pinned {
		return styles.pinned
	}
	return sessionRowStyle(styles, item)
}

func sessionMarker(item SessionItem) string {
	if item.Attention {
		return attentionMarkerSymbol
	}
	if item.Current {
		return currentMarkerSymbol
	}
	if item.Pinned {
		return pinnedMarkerSymbol
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
		var row string
		if i == m.cursor {
			row = styles.selected.Render(fmt.Sprintf("%s %s%s", sessionMarker(item), badge, item.Name))
		} else {
			marker := sessionMarkerStyle(styles, item).Render(sessionMarker(item))
			body := sessionRowStyle(styles, item).Render(fmt.Sprintf("%s%s", badge, item.Name))
			row = marker + " " + body
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

func pinnedToggleKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return key.Mod == 0 && (key.Text == " " || key.Code == tea.KeySpace)
}

func numericSlotKey(msg tea.KeyPressMsg) (int, bool) {
	key := msg.Key()
	if key.Mod != 0 {
		return 0, false
	}
	if key.Text == "0" || key.Code == '0' {
		return 10, true
	}
	for slot := 1; slot <= 9; slot++ {
		digit := rune('0' + slot)
		if key.Text == string(digit) || key.Code == digit {
			return slot, true
		}
	}
	return 0, false
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
	return fmt.Sprintf("%d", slot)
}
