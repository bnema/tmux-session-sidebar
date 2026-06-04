package uity

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

const attentionMarkerSymbol = "\uf0f3" // Nerd Font bell glyph (U+F0F3 / nf-fa-bell).
const currentMarkerSymbol = "\uf444"   // Nerd Font dot-fill glyph (U+F444 / nf-oct-dot_fill).
const pinnedMarkerSymbol = "\uf08d"    // Nerd Font thumb-tack glyph (U+F08D / nf-fa-thumb_tack).
const spaceKeySymbol = "\U000F1050"    // Nerd Font keyboard-space glyph (U+F1050 / nf-md-keyboard_space).
const updateAvailableSymbol = "\uf062" // Nerd Font arrow-up glyph (U+F062 / nf-fa-arrow_up).

const metadataSublinePaddingWidth = 6
const metadataSublineFallbackWidth = 80

type SessionItem struct {
	Name          string
	Current       bool
	Slot          int
	Heat          string
	HeatIntensity float64
	Attention     bool
	Pinned        bool
	PinColor      string
	Metadata      SessionMetadataSubline
}

type ProjectItem struct {
	Name string
	Path string
}

type TreeRowKind string

const (
	TreeRowCategory  TreeRowKind = "category"
	TreeRowSession   TreeRowKind = "session"
	TreeRowSeparator TreeRowKind = "separator"
	TreeRowSpacer    TreeRowKind = "spacer"
)

type TreeItem struct {
	Kind           TreeRowKind
	ID             string
	CategoryID     string
	CategoryName   string
	CategoryOpen   bool
	Session        SessionItem
	Slot           int
	Branch         string
	MetadataPrefix string
	ShowMetadata   bool
}

type Actions struct {
	SwitchSession       func(string) bool
	CreateProject       func(ProjectItem) bool
	CreateGitProject    func() bool
	CreateAdhoc         func() bool
	CreateNamedSession  func(string) bool
	RenameSession       func(string) bool
	KillSession         func(string) bool
	TogglePinnedSession func(string) bool
	PinSessionWithColor func(string, string) bool
	ReorderSession      func(string, int) bool
	SetShowNumericItems func(bool) bool
	SelfUpdate          func() tea.Cmd
	LoadProjects        func() []ProjectItem
	ReloadSessions      func() []SessionItem
	ReloadTreeItems     func() []TreeItem
	CreateCategory      func(string) bool
	RenameCategory      func(string, string) bool
	CreateSpacer        func() bool
	CreateSeparator     func() bool
	MoveTreeItem        func(string, int) bool
}

type SelfUpdateFinishedMsg struct {
	Err error
}

type SidebarOptions struct {
	ShowNumericItems        bool
	Version                 string
	CheckUpdateAvailable    func(currentVersion string) (bool, error)
	MetadataIconMode        MetadataIconMode
	AgentAttentionAnimation config.AgentAttentionAnimation
}

type SidebarModel struct {
	items                            []SessionItem
	treeItems                        []TreeItem
	treeMode                         bool
	cursor                           int
	mode                             Mode
	filter                           string
	showNumeric                      bool
	showHelp                         bool
	message                          string
	projects                         []ProjectItem
	projectCursor                    int
	projectFilter                    string
	pendingKill                      string
	renameCategoryID                 string
	renameCategoryInput              string
	createNamedInput                 string
	actions                          Actions
	version                          string
	updateCheck                      updateCheckState
	updateSpinner                    spinner.Model
	updateInProgress                 bool
	metadataIconMode                 MetadataIconMode
	width                            int
	height                           int
	pinColorPicker                   PinColorPicker
	pinColorSession                  string
	attentionAnimationStyle          config.AgentAttentionAnimation
	attentionAnimationFrame          int
	attentionAnimationTickPending    bool
	attentionAnimationTickGeneration int
}

type sidebarStyles struct {
	accent          lipgloss.Style
	dim             lipgloss.Style
	active          lipgloss.Style
	stale           lipgloss.Style
	selected        lipgloss.Style
	pinned          lipgloss.Style
	versionBadge    lipgloss.Style
	updateIndicator lipgloss.Style
}

func NewSidebarModel(items []SessionItem, actions Actions) SidebarModel {
	return NewSidebarModelWithOptions(items, actions, SidebarOptions{})
}

func NewTreeSidebarModelWithOptions(treeItems []TreeItem, actions Actions, options SidebarOptions) SidebarModel {
	items := sessionItemsFromTree(treeItems)
	model := NewSidebarModelWithOptions(items, actions, options)
	model.treeItems = treeItems
	model.treeMode = true
	return model
}

func NewSidebarModelWithOptions(items []SessionItem, actions Actions, options SidebarOptions) SidebarModel {
	iconMode := options.MetadataIconMode
	if iconMode == "" {
		iconMode = bestEffortMetadataIconMode()
	}
	attentionAnimationStyle := config.ParseAgentAttentionAnimation(string(options.AgentAttentionAnimation))
	attentionAnimationTickPending := shouldRunAttentionAnimation(attentionAnimationStyle, items)
	attentionAnimationTickGeneration := 0
	if attentionAnimationTickPending {
		attentionAnimationTickGeneration = 1
	}
	updateSpinner := spinner.New()
	updateSpinner.Spinner = spinner.Meter
	updateSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc"))
	return SidebarModel{
		items:                            items,
		treeItems:                        SessionItemsToTree(items),
		actions:                          actions,
		mode:                             ModeBrowse,
		showNumeric:                      options.ShowNumericItems,
		version:                          options.Version,
		updateCheck:                      newUpdateCheckState(options.Version, options.CheckUpdateAvailable),
		updateSpinner:                    updateSpinner,
		metadataIconMode:                 iconMode,
		attentionAnimationStyle:          attentionAnimationStyle,
		attentionAnimationTickPending:    attentionAnimationTickPending,
		attentionAnimationTickGeneration: attentionAnimationTickGeneration,
	}
}

func (m SidebarModel) Init() tea.Cmd {
	return batchCommands(m.updateCheck.initCmd(), attentionAnimationTickCmd(m.attentionAnimationStyle, m.items, m.attentionAnimationTickGeneration))
}

func (m SidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case updateAvailableMsg:
		m.updateCheck = m.updateCheck.handleResult(msg)
		return m, nil
	case updateCheckTickMsg:
		var cmd tea.Cmd
		m.updateCheck, cmd = m.updateCheck.handleTick()
		return m, cmd
	case spinner.TickMsg:
		if !m.updateInProgress {
			return m, nil
		}
		var cmd tea.Cmd
		m.updateSpinner, cmd = m.updateSpinner.Update(msg)
		return m, cmd
	case SelfUpdateFinishedMsg:
		m.updateInProgress = false
		if msg.Err != nil {
			m.message = "Update failed: " + msg.Err.Error()
		} else {
			m.message = "Update complete"
		}
		return m, nil
	case attentionAnimationTickMsg:
		if msg.Generation != untrackedAttentionAnimationTick && msg.Generation != m.attentionAnimationTickGeneration {
			return m, nil
		}
		m.attentionAnimationTickPending = false
		if !shouldRunAttentionAnimation(m.attentionAnimationStyle, m.items) {
			m.attentionAnimationFrame = 0
			return m, nil
		}
		m.attentionAnimationFrame = nextAttentionAnimationFrame(m.attentionAnimationStyle, m.attentionAnimationFrame)
		return m, m.startAttentionAnimationCmd()
	case tea.MouseWheelMsg:
		m.handleMouseWheel(msg)
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)
	}
	return m, nil
}

func (m SidebarModel) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Keystroke()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.mode == ModePinColor {
		m.handlePinColorKey(msg)
		return m.finishInteractiveUpdate()
	}
	if m.mode == ModeRenameCategory {
		m.handleRenameCategoryKey(msg)
		return m.finishInteractiveUpdate()
	}
	if m.mode == ModeCreateNamed {
		m.handleCreateNamedKey(msg)
		return m.finishInteractiveUpdate()
	}
	if m.mode == ModeBrowse {
		if delta, ok := reorderKeyDelta(msg); ok {
			m.reorderSelected(delta)
			return m.finishInteractiveUpdate()
		}
	}
	if m.mode == ModeConfirmKill {
		if isKillConfirmYes(msg) {
			m.confirmKill()
			return m.finishInteractiveUpdate()
		}
		if isKillConfirmCancel(msg) {
			m.clearKillConfirmation()
			return m.finishInteractiveUpdate()
		}
		return m, nil
	}
	if m.mode == ModeBrowse && toggleNumericKey(msg) {
		next := !m.showNumeric
		if m.actions.SetShowNumericItems != nil && !m.actions.SetShowNumericItems(next) {
			return m, nil
		}
		m.showNumeric = next
		if m.treeMode {
			selectedID := ""
			if item, ok := m.selectedTreeItem(); ok {
				selectedID = item.ID
			}
			m.reloadTreeItems()
			if selectedID != "" {
				m.selectTreeItem(selectedID)
			}
		}
		return m.finishInteractiveUpdate()
	}
	if pinnedToggleKey(msg) && m.mode == ModeBrowse {
		m.handlePinKey()
		return m.finishInteractiveUpdate()
	}
	if slot, ok := numericSlotKey(msg); ok && m.mode == ModeBrowse {
		m.switchSlot(slot)
		return m.finishInteractiveUpdate()
	}
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.mode == ModeSearch {
			m.mode = ModeBrowse
			m.filter = ""
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeProject {
			m.mode = ModeBrowse
			m.projectFilter = ""
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeNewItem || m.mode == ModeCreateSession {
			m.mode = ModeBrowse
			m.projectCursor = 0
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeCreateNamed {
			m.clearCreateNamed()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeRenameCategory {
			m.clearRenameCategory()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeConfirmKill {
			m.clearKillConfirmation()
			return m.finishInteractiveUpdate()
		}
		return m, tea.Quit
	case "/":
		if m.mode == ModeBrowse {
			m.mode = ModeSearch
		}
	case "enter":
		if m.mode == ModeSearch {
			m.mode = ModeBrowse
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeProject {
			m.createSelectedProject()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeNewItem {
			m.createSelectedNewItem()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeCreateSession {
			m.createSelectedSessionChoice()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeConfirmKill {
			m.clearKillConfirmation()
			return m.finishInteractiveUpdate()
		}
		m.switchSelected()
	case "j", "down":
		if m.mode == ModeProject || m.mode == ModeNewItem || m.mode == ModeCreateSession {
			m.moveProject(1)
		} else {
			m.move(1)
		}
	case "k", "up":
		if m.mode == ModeProject || m.mode == ModeNewItem || m.mode == ModeCreateSession {
			m.moveProject(-1)
		} else {
			m.move(-1)
		}
	case "n":
		if m.mode == ModeBrowse {
			if m.treeMode {
				m.projects = newItemMenuItems()
				m.mode = ModeNewItem
			} else {
				if m.actions.LoadProjects != nil {
					m.projects = m.actions.LoadProjects()
				}
				m.mode = ModeProject
			}
			m.projectCursor = 0
			m.projectFilter = ""
		} else {
			m.appendPrintable(msg)
		}
	case "c":
		if m.mode == ModeBrowse {
			m.projects = createSessionMenuItems()
			m.mode = ModeCreateSession
			m.projectCursor = 0
			m.projectFilter = ""
		} else {
			m.appendPrintable(msg)
		}
	case "g":
		if m.mode == ModeBrowse {
			if m.actions.CreateGitProject != nil && m.actions.CreateGitProject() {
				m.reloadSessionsSelectingCurrent()
			}
		} else {
			m.appendPrintable(msg)
		}
	case "a":
		if m.mode == ModeBrowse {
			if m.actions.CreateAdhoc != nil && m.actions.CreateAdhoc() {
				m.reloadSessionsSelectingCurrent()
			}
		} else {
			m.appendPrintable(msg)
		}
	case "r":
		if m.mode == ModeBrowse {
			if m.treeMode {
				if item, ok := m.selectedTreeItem(); ok && item.Kind == TreeRowCategory {
					m.mode = ModeRenameCategory
					m.renameCategoryID = item.ID
					m.renameCategoryInput = item.CategoryName
				}
				return m.finishInteractiveUpdate()
			}
			if item, ok := m.selectedSession(); ok && m.actions.RenameSession != nil && m.actions.RenameSession(item.Name) {
				m.reloadSessions()
			}
		} else {
			m.appendPrintable(msg)
		}
	case "x":
		if m.mode == ModeBrowse {
			if item, ok := m.selectedSession(); ok && m.actions.KillSession != nil {
				m.mode = ModeConfirmKill
				m.pendingKill = item.Name
				m.message = "Kill " + item.Name + "? y/N"
			}
		} else {
			m.appendPrintable(msg)
		}
	case "y":
		m.appendPrintable(msg)
	case "f5":
		m.reloadSessions()
	case "u":
		if m.mode == ModeBrowse {
			if m.updateInProgress {
				return m, nil
			}
			if m.actions.SelfUpdate != nil {
				updateCmd := m.actions.SelfUpdate()
				if updateCmd != nil {
					m.updateInProgress = true
					m.message = "Updating runtime " + m.updateSpinner.View()
					return m, tea.Batch(updateCmd, m.updateSpinner.Tick)
				}
			}
		} else {
			m.appendPrintable(msg)
		}
	case "?":
		if m.mode == ModeBrowse {
			m.showHelp = !m.showHelp
		} else {
			m.appendPrintable(msg)
		}
	case "backspace":
		if m.mode == ModeSearch && m.filter != "" {
			m.filter = trimLastRune(m.filter)
		}
		if (m.mode == ModeProject || m.mode == ModeCreateSession) && m.projectFilter != "" {
			m.projectFilter = trimLastRune(m.projectFilter)
		}
	default:
		m.appendPrintable(msg)
	}
	return m.finishInteractiveUpdate()
}

func (m *SidebarModel) finishInteractiveUpdate() (tea.Model, tea.Cmd) {
	return *m, m.startAttentionAnimationCmd()
}

func (m *SidebarModel) move(delta int) {
	if m.treeMode {
		visible := m.selectableTreeItems()
		if len(visible) == 0 {
			m.cursor = 0
			return
		}
		m.cursor = (m.cursor + delta + len(visible)) % len(visible)
		return
	}
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
	if m.treeMode {
		item, ok := m.selectedTreeItem()
		if !ok || item.Kind != TreeRowSession {
			return SessionItem{}, false
		}
		return item.Session, true
	}
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
	if m.treeMode {
		for _, item := range m.visibleTreeItems() {
			if item.Kind == TreeRowSession && item.Slot == slot {
				m.switchItem(item.Session)
				return
			}
		}
		return
	}
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
	if m.treeMode {
		m.reloadTreeItems()
		return
	}
	if m.actions.ReloadSessions == nil {
		return
	}
	// Preserve selection on the previously-current session when a refresh observes
	// a session switch, so the sidebar is ready to toggle back to where the user came from.
	previous := m.currentSessionName()
	m.items = m.actions.ReloadSessions()
	m.treeItems = SessionItemsToTree(m.items)
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

func (m *SidebarModel) startAttentionAnimationCmd() tea.Cmd {
	if m.attentionAnimationTickPending || !shouldRunAttentionAnimation(m.attentionAnimationStyle, m.items) {
		return nil
	}
	m.attentionAnimationTickGeneration++
	m.attentionAnimationTickPending = true
	return attentionAnimationTickCmd(m.attentionAnimationStyle, m.items, m.attentionAnimationTickGeneration)
}

func (m *SidebarModel) handlePinKey() tea.Cmd {
	item, ok := m.selectedSession()
	if !ok {
		return nil
	}
	if item.Pinned {
		m.togglePinnedSelected()
		return nil
	}
	m.mode = ModePinColor
	m.pinColorSession = item.Name
	m.pinColorPicker = PinColorPicker{}
	return nil
}

func (m *SidebarModel) handlePinColorKey(msg tea.KeyPressMsg) tea.Cmd {
	if delta, ok := m.pinColorPicker.MoveDelta(msg); ok {
		m.pinColorPicker.Cursor = m.pinColorPicker.Move(delta)
		return nil
	}
	if pinnedToggleKey(msg) || msg.Keystroke() == "enter" {
		m.confirmPinColor()
		return nil
	}
	if msg.Keystroke() == "esc" {
		m.clearPinColorPicker()
	}
	return nil
}

func (m *SidebarModel) confirmPinColor() {
	name := m.pinColorSession
	color := m.pinColorPicker.SelectedColor()
	if name == "" || m.actions.PinSessionWithColor == nil || !m.actions.PinSessionWithColor(name, color) {
		return
	}
	m.clearPinColorPicker()
	m.reloadSessions()
	m.selectSession(name)
}

func (m *SidebarModel) clearPinColorPicker() {
	m.mode = ModeBrowse
	m.pinColorSession = ""
	m.pinColorPicker = PinColorPicker{}
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
	if m.treeMode {
		item, ok := m.selectedTreeItem()
		if !ok || m.actions.MoveTreeItem == nil || !m.actions.MoveTreeItem(item.ID, delta) {
			return
		}
		m.reloadTreeItems()
		m.selectTreeItem(item.ID)
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
	view := tea.NewView(m.Render())
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m SidebarModel) Render() string {
	styles := newSidebarStyles()
	lines := []string{""}
	if m.mode == ModeProject {
		lines = append(lines, m.renderProjects(styles)...)
	} else if m.treeMode {
		lines = append(lines, m.renderTree(styles)...)
	} else {
		lines = append(lines, m.renderSessions(styles)...)
	}
	if status := m.statusLine(); status != "" {
		lines = append(lines, "", styles.accent.Render(status))
	}
	if m.updateInProgress {
		lines = append(lines, "", styles.accent.Render("Updating runtime "+m.updateSpinner.View()))
	} else if m.message != "" {
		lines = append(lines, "", styles.accent.Render(m.message))
	}
	lines = StatusBar{Lines: m.statusBarLines(styles), Height: m.height}.RenderBelow(padSidebarContentLines(lines))
	content := strings.Join(lines, "\n")
	if m.mode == ModePinColor {
		return m.pinColorPicker.RenderOverlay(content, m.width, m.height)
	}
	if m.mode == ModeNewItem {
		return m.renderBottomSheet(content, BottomSheet{Title: "new layout item", Content: m.renderMenuContent(styles), Footer: "esc cancel  ↵ create", Height: 7})
	}
	if m.mode == ModeCreateSession {
		return m.renderBottomSheet(content, BottomSheet{Title: "create session", Content: m.renderMenuContent(styles), Footer: "esc cancel  ↵ choose", Height: 8})
	}
	if m.mode == ModeCreateNamed {
		return m.renderBottomSheet(content, BottomSheet{Title: "named session", Content: "> " + m.createNamedInput, Footer: "esc cancel  ↵ create", Height: 5})
	}
	return content
}

func (m *SidebarModel) handleMouseWheel(msg tea.MouseWheelMsg) {
	mouse := msg.Mouse()
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.moveWheel(-1)
	case tea.MouseWheelDown:
		m.moveWheel(1)
	}
}

func (m *SidebarModel) moveWheel(delta int) {
	if m.mode == ModeConfirmKill {
		return
	}
	if m.mode == ModeProject || m.mode == ModeNewItem || m.mode == ModeCreateSession {
		m.moveProject(delta)
		return
	}
	m.move(delta)
}

func reorderKeyDelta(msg tea.KeyPressMsg) (int, bool) {
	switch msg.Key().Text {
	case "J":
		return 1, true
	case "K":
		return -1, true
	default:
		return 0, false
	}
}

func toggleNumericKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return key.Mod == 0 && key.Text == "h"
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
