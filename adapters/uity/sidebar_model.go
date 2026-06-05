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
const pinnedMarkerSymbol = "\uf08d"    // Nerd Font thumb-tack glyph (U+F08D / nf-fa-thumb_tack).
const spaceKeySymbol = "\U000F1050"    // Nerd Font keyboard-space glyph (U+F1050 / nf-md-keyboard_space).
const updateAvailableSymbol = "\uf062" // Nerd Font arrow-up glyph (U+F062 / nf-fa-arrow_up).

const metadataSublinePaddingWidth = 6
const metadataSublineFallbackWidth = 80
const metadataSublineSidebarFallbackWidth = 30

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
	TreeRowMore      TreeRowKind = "more"
)

type TreeItem struct {
	Kind           TreeRowKind
	ID             string
	CategoryID     string
	CategoryName   string
	CategoryOpen   bool
	Color          string
	Session        SessionItem
	Slot           int
	Depth          int
	LastChild      bool
	ShowMetadata   bool
	MoreCount      int
	MoreExpanded   bool
	OverflowHidden bool
}

type Actions struct {
	SwitchSession               func(string) bool
	CreateProject               func(ProjectItem, string) bool
	CreateGitProject            func(string) bool
	CreateAdhoc                 func(string) bool
	CreateNamedSession          func(string, string) bool
	KillSession                 func(string) bool
	TogglePinnedSession         func(string) bool
	ColorSession                func(string, string) bool
	ColorCategory               func(string, string) bool
	SetShowNumericItems         func(bool) bool
	SelfUpdate                  func() tea.Cmd
	LoadProjects                func() []ProjectItem
	ReloadTreeItems             func() []TreeItem
	CreateCategory              func(string) bool
	RenameCategory              func(string, string) bool
	CreateSpacer                func() bool
	CreateSeparator             func() bool
	MoveTreeItem                func(string, int) bool
	DeleteTreeItem              func(TreeItem) bool
	SetCategoryCollapsed        func(string, bool) bool
	SetCategorySessionsExpanded func(string, bool) bool
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
	treeScroll                       int
	pinColorPicker                   PinColorPicker
	colorTarget                      colorTarget
	attentionAnimationStyle          config.AgentAttentionAnimation
	attentionAnimationFrame          int
	attentionAnimationTickPending    bool
	attentionAnimationTickGeneration int
}

type colorTarget struct {
	SessionName string
	CategoryID  string
	ItemID      string
}

type sidebarStyles struct {
	accent          lipgloss.Style
	dim             lipgloss.Style
	treeGuide       lipgloss.Style
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
		if msg.Width <= 0 || msg.Height <= 0 {
			return m, nil
		}
		m.width = msg.Width
		m.height = msg.Height
		m.ensureTreeCursorVisible()
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
	if msg.Keystroke() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case ModePinColor:
		m.handlePinColorKey(msg)
		return m.finishInteractiveUpdate()
	case ModeRenameCategory:
		m.handleRenameCategoryKey(msg)
		return m.finishInteractiveUpdate()
	case ModeCreateNamed:
		m.handleCreateNamedKey(msg)
		return m.finishInteractiveUpdate()
	case ModeCreateCategory:
		m.handleCreateCategoryKey(msg)
		return m.finishInteractiveUpdate()
	case ModeConfirmKill:
		return m.updateKillConfirmationKey(msg)
	case ModeConfirmDelete:
		return m.updateDeleteConfirmationKey(msg)
	case ModeSearch:
		return m.updateSearchKey(msg)
	case ModeCreate, ModeProject:
		return m.updateMenuKey(msg)
	case ModeBrowse:
		return m.updateBrowseKey(msg)
	default:
		return m, nil
	}
}

func (m SidebarModel) updateKillConfirmationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m SidebarModel) updateDeleteConfirmationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m SidebarModel) updateSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if delta, ok := pageKeyDelta(msg); ok {
		m.movePage(delta)
		return m.finishInteractiveUpdate()
	}
	switch msg.Keystroke() {
	case "esc":
		m.mode = ModeBrowse
		m.filter = ""
	case "enter":
		m.mode = ModeBrowse
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "backspace":
		if m.filter != "" {
			m.filter = trimLastRune(m.filter)
		}
	case "f5":
		m.reloadSessions()
	case "/":
		// Preserve previous behavior: slash opens search from browse, but is not added to the search text.
	default:
		m.appendPrintable(msg)
	}
	return m.finishInteractiveUpdate()
}

func (m SidebarModel) updateMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Keystroke() {
	case "esc":
		m.closeMenu()
	case "enter":
		m.chooseMenuItem()
	case "j", "down":
		m.moveMenu(1)
	case "k", "up":
		m.moveMenu(-1)
	case "backspace":
		m.backspaceMenuFilter()
	case "f5":
		m.reloadSessions()
	case "/":
		// Preserve previous behavior: slash is ignored while a menu is open.
	default:
		m.appendPrintable(msg)
	}
	return m.finishInteractiveUpdate()
}

func (m SidebarModel) updateBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if delta, ok := pageKeyDelta(msg); ok {
		m.movePage(delta)
		return m.finishInteractiveUpdate()
	}
	if delta, ok := reorderKeyDelta(msg); ok {
		m.reorderSelected(delta)
		return m.finishInteractiveUpdate()
	}
	if collapsed, ok := categoryCollapseKey(msg); ok && m.setSelectedCategoryCollapsed(collapsed) {
		return m.finishInteractiveUpdate()
	}
	if toggleNumericKey(msg) {
		return m.toggleNumericItems()
	}
	if pinnedToggleKey(msg) {
		m.handlePinKey()
		return m.finishInteractiveUpdate()
	}
	if colorizeKey(msg) {
		m.startColorPicker()
		return m.finishInteractiveUpdate()
	}
	if slot, ok := numericSlotKey(msg); ok {
		m.switchSlot(slot)
		return m.finishInteractiveUpdate()
	}

	switch msg.Keystroke() {
	case "esc":
		if m.showHelp {
			m.showHelp = false
			return m.finishInteractiveUpdate()
		}
		return m, tea.Quit
	case "/":
		m.mode = ModeSearch
	case "enter":
		m.activateSelected()
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "n":
		m.startCreateNamed()
	case "c":
		m.openCreateMenu()
	case "g":
		if m.actions.CreateGitProject != nil && m.actions.CreateGitProject(m.selectedCategoryID()) {
			m.reloadSessionsSelectingCurrent()
		}
	case "a":
		if m.actions.CreateAdhoc != nil && m.actions.CreateAdhoc(m.selectedCategoryID()) {
			m.reloadSessionsSelectingCurrent()
		}
	case "r":
		m.startRenameSelectedCategory()
		return m.finishInteractiveUpdate()
	case "x":
		m.openKillConfirmation()
	case "d":
		m.openDeleteConfirmation()
	case "f5":
		m.reloadSessions()
	case "u":
		return m.startSelfUpdate()
	case "?":
		m.showHelp = !m.showHelp
	default:
		m.appendPrintable(msg)
	}
	return m.finishInteractiveUpdate()
}

func (m *SidebarModel) toggleNumericItems() (tea.Model, tea.Cmd) {
	next := !m.showNumeric
	if m.actions.SetShowNumericItems != nil && !m.actions.SetShowNumericItems(next) {
		return *m, nil
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

func (m *SidebarModel) startRenameSelectedCategory() {
	if item, ok := m.selectedTreeItem(); ok && item.Kind == TreeRowCategory {
		m.mode = ModeRenameCategory
		m.renameCategoryID = item.ID
		m.renameCategoryInput = item.CategoryName
	}
}

func (m *SidebarModel) openKillConfirmation() {
	if item, ok := m.selectedSession(); ok && m.actions.KillSession != nil {
		m.mode = ModeConfirmKill
		m.pendingKill = item.Name
		m.message = "Kill " + item.Name + "? y/N"
	}
}

func (m *SidebarModel) startSelfUpdate() (tea.Model, tea.Cmd) {
	if m.updateInProgress {
		return *m, nil
	}
	if m.actions.SelfUpdate == nil {
		return m.finishInteractiveUpdate()
	}
	updateCmd := m.actions.SelfUpdate()
	if updateCmd == nil {
		return m.finishInteractiveUpdate()
	}
	m.updateInProgress = true
	m.message = "Updating runtime " + m.updateSpinner.View()
	m.ensureTreeCursorVisible()
	return *m, tea.Batch(updateCmd, m.updateSpinner.Tick)
}

func (m *SidebarModel) finishInteractiveUpdate() (tea.Model, tea.Cmd) {
	m.ensureTreeCursorVisible()
	return *m, m.startAttentionAnimationCmd()
}

func (m *SidebarModel) move(delta int) {
	visible := m.selectableTreeItems()
	if len(visible) == 0 {
		m.cursor = 0
		m.treeScroll = 0
		return
	}
	m.cursor = (m.cursor + delta + len(visible)) % len(visible)
	m.ensureTreeCursorVisible()
}

func (m *SidebarModel) movePage(delta int) {
	visible := m.selectableTreeItems()
	if len(visible) == 0 {
		m.cursor = 0
		m.treeScroll = 0
		return
	}
	step := m.availableTreeHeight()
	if step <= 0 {
		step = 1
	}
	direction := 1
	if delta < 0 {
		direction = -1
	}
	m.cursor = min(max(m.cursor+m.pageItemDelta(visible, step, direction, newSidebarStyles()), 0), len(visible)-1)
	m.ensureTreeCursorVisible()
}

func (m SidebarModel) pageItemDelta(visible []TreeItem, step int, direction int, styles sidebarStyles) int {
	if direction == 0 || len(visible) == 0 {
		return 0
	}
	renderer := m.treeLineCounter(styles)
	lines := 0
	items := 0
	for index := m.cursor + direction; index >= 0 && index < len(visible); index += direction {
		items += direction
		lines += renderer.renderedTreeItemLineCount(visible[index])
		if lines >= step {
			break
		}
	}
	return items
}

func (m SidebarModel) selectedSession() (SessionItem, bool) {
	item, ok := m.selectedTreeItem()
	if !ok || item.Kind != TreeRowSession {
		return SessionItem{}, false
	}
	return item.Session, true
}

func (m *SidebarModel) activateSelected() {
	item, ok := m.selectedTreeItem()
	if !ok {
		return
	}
	if item.Kind == TreeRowMore {
		m.toggleSelectedMore(item)
		return
	}
	if item.Kind == TreeRowSession {
		m.switchItem(item.Session)
	}
}

func (m *SidebarModel) toggleSelectedMore(item TreeItem) {
	if m.actions.SetCategorySessionsExpanded == nil || item.CategoryID == "" {
		return
	}
	next := !item.MoreExpanded
	if !m.actions.SetCategorySessionsExpanded(item.CategoryID, next) {
		return
	}
	m.setLocalCategorySessionsExpanded(item.CategoryID, next)
	m.reloadTreeItems()
	m.selectTreeItem(item.ID)
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
	if m.actions.SwitchSession(item.Name) && m.reloadTreeItems() {
		if !m.selectSessionIfExists(item.Name) {
			m.selectSession(m.currentSessionName())
		}
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
	m.togglePinnedSelected()
	return nil
}

func (m *SidebarModel) startColorPicker() {
	item, ok := m.selectedTreeItem()
	if !ok {
		return
	}
	switch item.Kind {
	case TreeRowSession:
		m.mode = ModePinColor
		m.colorTarget = colorTarget{SessionName: item.Session.Name, ItemID: item.ID}
		m.pinColorPicker = PinColorPicker{}
	case TreeRowCategory:
		m.mode = ModePinColor
		m.colorTarget = colorTarget{CategoryID: item.CategoryID, ItemID: item.ID}
		m.pinColorPicker = PinColorPicker{}
	}
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
	target := m.colorTarget
	color := m.pinColorPicker.SelectedColor()
	if target.SessionName != "" {
		if m.actions.ColorSession == nil || !m.actions.ColorSession(target.SessionName, color) {
			return
		}
		m.clearPinColorPicker()
		m.reloadSessions()
		m.selectSession(target.SessionName)
		return
	}
	if target.CategoryID != "" {
		if m.actions.ColorCategory == nil || !m.actions.ColorCategory(target.CategoryID, color) {
			return
		}
		m.clearPinColorPicker()
		m.reloadTreeItems()
		m.selectTreeItem(target.ItemID)
	}
}

func (m *SidebarModel) clearPinColorPicker() {
	m.mode = ModeBrowse
	m.colorTarget = colorTarget{}
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
		case TreeRowMore:
			return item.CategoryID
		case TreeRowSeparator, TreeRowSpacer:
			return m.nearestVisibleCategoryID(item.ID)
		}
	}
	return ""
}

func (m SidebarModel) nearestVisibleCategoryID(itemID string) string {
	visible := m.visibleTreeItems()
	selectedIndex := -1
	for i, item := range visible {
		if item.ID == itemID {
			selectedIndex = i
			break
		}
	}
	if selectedIndex < 0 {
		return ""
	}
	for i := selectedIndex - 1; i >= 0; i-- {
		if visible[i].CategoryID != "" {
			return visible[i].CategoryID
		}
	}
	for i := selectedIndex + 1; i < len(visible); i++ {
		if visible[i].CategoryID != "" {
			return visible[i].CategoryID
		}
	}
	return ""
}

func (m *SidebarModel) selectSession(name string) {
	if !m.selectSessionIfExists(name) {
		m.cursor = 0
	}
}

func (m *SidebarModel) selectSessionIfExists(name string) bool {
	selectable := m.selectableTreeItems()
	for i, item := range selectable {
		if item.Kind == TreeRowSession && item.Session.Name == name {
			m.cursor = i
			return true
		}
	}
	return false
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
	if !ok || item.Kind == TreeRowMore || m.actions.DeleteTreeItem == nil {
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
	lines = append(lines, m.renderScrollableTree(styles)...)
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
		return m.pinColorPicker.RenderOverlayAt(content, m.width, m.height, m.colorPickerOverlayY())
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

func pageKeyDelta(msg tea.KeyPressMsg) (int, bool) {
	switch msg.Key().Code {
	case tea.KeyPgDown:
		return 1, true
	case tea.KeyPgUp:
		return -1, true
	default:
		return 0, false
	}
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

func colorizeKey(msg tea.KeyPressMsg) bool {
	return msg.Key().Text == "C"
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
