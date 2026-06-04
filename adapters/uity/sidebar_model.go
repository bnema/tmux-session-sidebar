package uity

// Keep this file intentionally small.
// SidebarModel should stay a thin Bubble Tea coordinator: state, key routing,
// and calls into focused helpers/actions only. Move rendering, prompts, menu
// components, layout helpers, and business/persistence logic into separate
// files or packages instead of growing this file.

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

const attentionMarkerSymbol = "\uf0f3" // Nerd Font bell glyph (U+F0F3 / nf-fa-bell).
const currentMarkerSymbol = "\uf444"   // Nerd Font dot-fill glyph (U+F444 / nf-oct-dot_fill).
const inactiveMarkerSymbol = "\uf4c3"  // Nerd Font small hollow dot glyph (U+F4C3 / nf-oct-dot).
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
	Kind         TreeRowKind
	ID           string
	CategoryID   string
	CategoryName string
	CategoryOpen bool
	Session      SessionItem
	Slot         int
	Depth        int
	LastChild    bool
	ShowMetadata bool
}

type Actions struct {
	SwitchSession        func(string) bool
	CreateProject        func(ProjectItem, string) bool
	CreateGitProject     func(string) bool
	CreateAdhoc          func(string) bool
	CreateNamedSession   func(string, string) bool
	RenameSession        func(string) bool
	KillSession          func(string) bool
	TogglePinnedSession  func(string) bool
	PinSessionWithColor  func(string, string) bool
	SetShowNumericItems  func(bool) bool
	SelfUpdate           func() tea.Cmd
	LoadProjects         func() []ProjectItem
	ReloadTreeItems      func() []TreeItem
	CreateCategory       func(string) bool
	RenameCategory       func(string, string) bool
	CreateSpacer         func() bool
	CreateSeparator      func() bool
	MoveTreeItem         func(string, int) bool
	DeleteTreeItem       func(TreeItem) bool
	SetCategoryCollapsed func(string, bool) bool
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
	treeItems                        []TreeItem
	cursor                           int
	mode                             Mode
	filter                           string
	showNumeric                      bool
	showHelp                         bool
	message                          string
	menu                             menuState
	pendingKill                      string
	pendingDelete                    TreeItem
	renameCategoryID                 string
	renameCategoryInput              string
	createNamedInput                 string
	createCategoryInput              string
	createTargetCategoryID           string
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
	warning         lipgloss.Style
	destructive     lipgloss.Style
	versionBadge    lipgloss.Style
	updateIndicator lipgloss.Style
}

func NewTreeSidebarModelWithOptions(treeItems []TreeItem, actions Actions, options SidebarOptions) SidebarModel {
	iconMode := options.MetadataIconMode
	if iconMode == "" {
		iconMode = bestEffortMetadataIconMode()
	}
	attentionAnimationStyle := config.ParseAgentAttentionAnimation(string(options.AgentAttentionAnimation))
	attentionAnimationTickPending := shouldRunAttentionAnimation(attentionAnimationStyle, sessionItemsFromTree(treeItems))
	attentionAnimationTickGeneration := 0
	if attentionAnimationTickPending {
		attentionAnimationTickGeneration = 1
	}
	updateSpinner := spinner.New()
	updateSpinner.Spinner = spinner.Meter
	updateSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc"))
	return SidebarModel{
		treeItems:                        treeItems,
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
	return batchCommands(m.updateCheck.initCmd(), attentionAnimationTickCmd(m.attentionAnimationStyle, sessionItemsFromTree(m.treeItems), m.attentionAnimationTickGeneration))
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
		if !shouldRunAttentionAnimation(m.attentionAnimationStyle, sessionItemsFromTree(m.treeItems)) {
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
	if m.mode == ModeCreateCategory {
		m.handleCreateCategoryKey(msg)
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
	if m.mode == ModeConfirmDelete {
		if isDeleteConfirmYes(msg) {
			m.confirmDelete()
			return m.finishInteractiveUpdate()
		}
		if isDeleteConfirmCancel(msg) {
			m.clearDeleteConfirmation()
			return m.finishInteractiveUpdate()
		}
		return m, nil
	}
	if m.mode == ModeBrowse {
		if collapsed, ok := categoryCollapseKey(msg); ok && m.setSelectedCategoryCollapsed(collapsed) {
			return m.finishInteractiveUpdate()
		}
	}
	if m.mode == ModeBrowse && toggleNumericKey(msg) {
		next := !m.showNumeric
		if m.actions.SetShowNumericItems != nil && !m.actions.SetShowNumericItems(next) {
			return m, nil
		}
		m.showNumeric = next
		selectedID := ""
		if item, ok := m.selectedTreeItem(); ok {
			selectedID = item.ID
		}
		m.reloadTreeItems()
		if selectedID != "" {
			m.selectTreeItem(selectedID)
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
		if m.showHelp && m.mode == ModeBrowse {
			m.showHelp = false
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeSearch {
			m.mode = ModeBrowse
			m.filter = ""
			return m.finishInteractiveUpdate()
		}
		if m.menuActive() {
			m.closeMenu()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeCreateNamed {
			m.clearCreateNamed()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeCreateCategory {
			m.clearCreateCategory()
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
		if m.menuActive() {
			m.chooseMenuItem()
			return m.finishInteractiveUpdate()
		}
		if m.mode == ModeConfirmKill {
			m.clearKillConfirmation()
			return m.finishInteractiveUpdate()
		}
		m.switchSelected()
	case "j", "down":
		if m.menuActive() {
			m.moveMenu(1)
		} else {
			m.move(1)
		}
	case "k", "up":
		if m.menuActive() {
			m.moveMenu(-1)
		} else {
			m.move(-1)
		}
	case "n":
		if m.mode == ModeBrowse {
			m.startCreateNamed()
		} else {
			m.appendPrintable(msg)
		}
	case "c":
		if m.mode == ModeBrowse {
			m.openCreateMenu()
		} else {
			m.appendPrintable(msg)
		}
	case "g":
		if m.mode == ModeBrowse {
			if m.actions.CreateGitProject != nil && m.actions.CreateGitProject(m.selectedCategoryID()) {
				m.reloadSessionsSelectingCurrent()
			}
		} else {
			m.appendPrintable(msg)
		}
	case "a":
		if m.mode == ModeBrowse {
			if m.actions.CreateAdhoc != nil && m.actions.CreateAdhoc(m.selectedCategoryID()) {
				m.reloadSessionsSelectingCurrent()
			}
		} else {
			m.appendPrintable(msg)
		}
	case "r":
		if m.mode == ModeBrowse {
			if item, ok := m.selectedTreeItem(); ok && item.Kind == TreeRowCategory {
				m.mode = ModeRenameCategory
				m.renameCategoryID = item.ID
				m.renameCategoryInput = item.CategoryName
			}
			return m.finishInteractiveUpdate()
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
	case "d":
		if m.mode == ModeBrowse {
			m.openDeleteConfirmation()
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
		if m.menuActive() {
			m.backspaceMenuFilter()
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
	visible := m.selectableTreeItems()
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = (m.cursor + delta + len(visible)) % len(visible)
}

func (m SidebarModel) selectedSession() (SessionItem, bool) {
	item, ok := m.selectedTreeItem()
	if !ok || item.Kind != TreeRowSession {
		return SessionItem{}, false
	}
	return item.Session, true
}

func (m *SidebarModel) switchSelected() {
	item, ok := m.selectedSession()
	if !ok {
		return
	}
	m.switchItem(item)
}

func (m *SidebarModel) switchSlot(slot int) {
	for _, item := range m.visibleTreeItems() {
		if item.Kind == TreeRowSession && item.Slot == slot {
			m.switchItem(item.Session)
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
	for _, item := range sessionItemsFromTree(m.treeItems) {
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
	previous := m.currentSessionName()
	m.reloadTreeItems()
	current := m.currentSessionName()
	if preservePreviousCurrent && previous != "" && previous != current {
		m.selectSession(previous)
		return
	}
	if !preservePreviousCurrent && current != "" {
		m.selectSession(current)
	}
}

func (m *SidebarModel) startAttentionAnimationCmd() tea.Cmd {
	if m.attentionAnimationTickPending || !shouldRunAttentionAnimation(m.attentionAnimationStyle, sessionItemsFromTree(m.treeItems)) {
		return nil
	}
	m.attentionAnimationTickGeneration++
	m.attentionAnimationTickPending = true
	return attentionAnimationTickCmd(m.attentionAnimationStyle, sessionItemsFromTree(m.treeItems), m.attentionAnimationTickGeneration)
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

func (m *SidebarModel) setSelectedCategoryCollapsed(collapsed bool) bool {
	item, ok := m.selectedTreeItem()
	if !ok || item.Kind != TreeRowCategory || m.actions.SetCategoryCollapsed == nil {
		return false
	}
	if !m.actions.SetCategoryCollapsed(item.CategoryID, collapsed) {
		return false
	}
	m.reloadTreeItems()
	m.selectTreeItem(item.ID)
	return true
}

func (m *SidebarModel) reorderSelected(delta int) {
	if m.mode != ModeBrowse && m.mode != ModeSearch {
		return
	}
	item, ok := m.selectedTreeItem()
	if !ok || m.actions.MoveTreeItem == nil || !m.actions.MoveTreeItem(item.ID, delta) {
		return
	}
	m.reloadTreeItems()
	m.selectTreeItem(item.ID)
}

func (m SidebarModel) selectedCategoryID() string {
	if item, ok := m.selectedTreeItem(); ok {
		switch item.Kind {
		case TreeRowCategory:
			return item.CategoryID
		case TreeRowSession:
			return item.CategoryID
		}
	}
	return ""
}

func (m *SidebarModel) selectSession(name string) {
	selectable := m.selectableTreeItems()
	for i, item := range selectable {
		if item.Kind == TreeRowSession && item.Session.Name == name {
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

func (m *SidebarModel) openDeleteConfirmation() {
	item, ok := m.selectedTreeItem()
	if !ok || m.actions.DeleteTreeItem == nil {
		return
	}
	m.mode = ModeConfirmDelete
	m.pendingDelete = item
	m.message = "Delete " + deleteLabel(item) + "? y/N"
}

func (m *SidebarModel) confirmDelete() {
	item := m.pendingDelete
	m.clearDeleteConfirmation()
	if item.ID != "" && m.actions.DeleteTreeItem != nil && m.actions.DeleteTreeItem(item) {
		m.reloadSessions()
	}
}

func (m *SidebarModel) clearDeleteConfirmation() {
	m.mode = ModeBrowse
	m.pendingDelete = TreeItem{}
	m.message = ""
}

func deleteLabel(item TreeItem) string {
	switch item.Kind {
	case TreeRowSession:
		return item.Session.Name
	case TreeRowCategory:
		return item.CategoryName
	case TreeRowSeparator:
		return "[separator]"
	case TreeRowSpacer:
		return "[spacer]"
	default:
		return "item"
	}
}

func (m *SidebarModel) appendPrintable(msg tea.KeyPressMsg) {
	if key, ok := printableKey(msg); ok {
		if m.mode == ModeSearch {
			m.filter += key
		}
		if m.menuActive() {
			m.appendMenuFilter(key)
		}
	}
}

func (m SidebarModel) View() tea.View {
	view := tea.NewView(m.Render())
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m SidebarModel) Render() string {
	styles := newSidebarStyles()
	lines := []string{""}
	lines = append(lines, m.renderTree(styles)...)
	if status := m.statusLine(); status != "" {
		lines = append(lines, "", styles.accent.Render(status))
	}
	if m.updateInProgress {
		lines = append(lines, "", styles.accent.Render("Updating runtime "+m.updateSpinner.View()))
	} else if m.message != "" {
		lines = append(lines, "", m.messageStyle(styles).Render(m.message))
	}
	lines = StatusBar{Lines: m.statusBarLines(styles), Height: m.height}.RenderBelow(padSidebarContentLines(lines))
	content := strings.Join(lines, "\n")
	if m.mode == ModePinColor {
		return m.pinColorPicker.RenderOverlay(content, m.width, m.height)
	}
	if m.menuActive() {
		return m.renderBottomSheet(content, bottomSheet{Title: m.menu.Spec.Title, Content: m.renderMenuRows(styles), Footer: m.menu.Spec.Footer, Height: m.menu.Spec.Height})
	}
	if m.mode == ModeCreateNamed {
		return m.renderBottomSheet(content, bottomSheet{Title: "new session", Content: "> " + m.createNamedInput, Footer: "esc cancel  ↵ create", Height: 5})
	}
	if m.mode == ModeCreateCategory {
		return m.renderBottomSheet(content, bottomSheet{Title: "new category", Content: "> " + m.createCategoryInput, Footer: "esc cancel  ↵ create", Height: 5})
	}
	if m.showHelp {
		return m.renderBottomSheet(content, bottomSheet{Title: "keys", Content: m.helpSheetContent(styles), Footer: "esc close", Height: 14})
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
	if m.mode == ModeConfirmKill || m.mode == ModeConfirmDelete {
		return
	}
	if m.menuActive() {
		m.moveMenu(delta)
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
	return key.Mod.Contains(tea.ModAlt) && key.Text == "h"
}

func categoryCollapseKey(msg tea.KeyPressMsg) (bool, bool) {
	switch msg.Keystroke() {
	case "h", "left":
		return true, true
	case "l", "right":
		return false, true
	default:
		return false, false
	}
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

func isDeleteConfirmYes(msg tea.KeyPressMsg) bool {
	return msg.Key().Text == "y" || msg.Key().Text == "Y"
}

func isDeleteConfirmCancel(msg tea.KeyPressMsg) bool {
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
