package sidebar

import (
	"fmt"
	"strings"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

const (
	DefaultCategoryID   = "category:default"
	DefaultCategoryName = "Default"
)

type Layout struct {
	Items []LayoutItem
}

type ItemKind string

const (
	ItemKindCategory  ItemKind = "category"
	ItemKindSeparator ItemKind = "separator"
	ItemKindSpacer    ItemKind = "spacer"
)

type LayoutItem struct {
	ID        string
	Kind      ItemKind
	Category  Category
	Separator Separator
	Spacer    Spacer
}

type Category struct {
	ID        string
	Name      string
	Collapsed bool
	Sessions  []SessionRef
}

type SessionRef struct {
	Name string
}

type Separator struct {
	ID string
}

type Spacer struct {
	ID string
}

type RowKind string

const (
	RowKindCategory  RowKind = "category"
	RowKindSession   RowKind = "session"
	RowKindSeparator RowKind = "separator"
	RowKindSpacer    RowKind = "spacer"
)

type Selection struct {
	Kind       RowKind
	ItemID     string
	CategoryID string
	Session    string
}

type TreeRow struct {
	Kind         RowKind
	ItemID       string
	CategoryID   string
	CategoryName string
	CategoryOpen bool
	Session      string
	Slot         int
	Depth        int
	LastChild    bool
}

func CategoryItem(id string, name string, collapsed bool, sessionNames []string) LayoutItem {
	id = normalizeID(id, "category", name)
	refs := make([]SessionRef, 0, len(sessionNames))
	for _, name := range sessionNames {
		if strings.TrimSpace(name) != "" {
			refs = append(refs, SessionRef{Name: name})
		}
	}
	return LayoutItem{ID: id, Kind: ItemKindCategory, Category: Category{ID: id, Name: name, Collapsed: collapsed, Sessions: refs}}
}

func SeparatorItem(id string) LayoutItem {
	id = normalizeID(id, "separator", "")
	return LayoutItem{ID: id, Kind: ItemKindSeparator, Separator: Separator{ID: id}}
}

func SpacerItem(id string) LayoutItem {
	id = normalizeID(id, "spacer", "")
	return LayoutItem{ID: id, Kind: ItemKindSpacer, Spacer: Spacer{ID: id}}
}

func EnsureLayout(existing Layout, live []string, order []string) Layout {
	live = sessions.ApplyOrder(validLiveNames(live), order)
	liveSet := stringSet(live)
	assigned := map[string]bool{}
	items := make([]LayoutItem, 0, max(1, len(existing.Items)))
	defaultIndex := -1
	for _, item := range existing.Items {
		switch item.Kind {
		case ItemKindCategory:
			category := normalizedCategory(item.Category)
			kept := make([]SessionRef, 0, len(category.Sessions))
			for _, ref := range category.Sessions {
				name := strings.TrimSpace(ref.Name)
				if name == "" || !liveSet[name] || assigned[name] {
					continue
				}
				kept = append(kept, SessionRef{Name: name})
				assigned[name] = true
			}
			category.Sessions = kept
			item.ID = category.ID
			item.Category = category
			items = append(items, item)
			if item.ID == DefaultCategoryID {
				defaultIndex = len(items) - 1
			}
		case ItemKindSeparator:
			items = append(items, SeparatorItem(itemID(item)))
		case ItemKindSpacer:
			items = append(items, SpacerItem(itemID(item)))
		}
	}
	if defaultIndex < 0 {
		items = append(items, CategoryItem(DefaultCategoryID, DefaultCategoryName, false, nil))
		defaultIndex = len(items) - 1
	}
	for _, name := range live {
		if !assigned[name] {
			items[defaultIndex].Category.Sessions = append(items[defaultIndex].Category.Sessions, SessionRef{Name: name})
			assigned[name] = true
		}
	}
	return Layout{Items: items}
}

func Flatten(layout Layout, selection Selection, showNumeric bool) []TreeRow {
	active := ActiveCategoryID(layout, selection)
	rows := make([]TreeRow, 0)
	for _, item := range layout.Items {
		switch item.Kind {
		case ItemKindCategory:
			category := normalizedCategory(item.Category)
			rows = append(rows, TreeRow{Kind: RowKindCategory, ItemID: category.ID, CategoryID: category.ID, CategoryName: category.Name, CategoryOpen: !category.Collapsed})
			if category.Collapsed {
				continue
			}
			slotByName := map[string]int{}
			if category.ID == active {
				slotByName = SlotMap(category.Sessions, showNumeric)
			}
			visibleSessions := visibleCategorySessions(category.Sessions, showNumeric)
			for i, ref := range visibleSessions {
				rows = append(rows, TreeRow{Kind: RowKindSession, ItemID: sessionItemID(category.ID, ref.Name), CategoryID: category.ID, Session: ref.Name, Slot: slotByName[ref.Name], Depth: 1, LastChild: i == len(visibleSessions)-1})
			}
		case ItemKindSeparator:
			rows = append(rows, TreeRow{Kind: RowKindSeparator, ItemID: itemID(item)})
		case ItemKindSpacer:
			rows = append(rows, TreeRow{Kind: RowKindSpacer, ItemID: itemID(item)})
		}
	}
	return rows
}

func SelectionForItemID(itemIDValue string) Selection {
	if categoryID, sessionName, ok := strings.Cut(itemIDValue, "/session:"); ok {
		return Selection{Kind: RowKindSession, ItemID: itemIDValue, CategoryID: categoryID, Session: sessionName}
	}
	kind, _, _ := strings.Cut(itemIDValue, ":")
	switch kind {
	case "category":
		return Selection{Kind: RowKindCategory, ItemID: itemIDValue, CategoryID: itemIDValue}
	case "separator":
		return Selection{Kind: RowKindSeparator, ItemID: itemIDValue}
	case "spacer":
		return Selection{Kind: RowKindSpacer, ItemID: itemIDValue}
	default:
		return Selection{ItemID: itemIDValue}
	}
}

func ActiveCategoryID(layout Layout, selection Selection) string {
	if selection.Kind == RowKindCategory && selection.CategoryID != "" {
		return selection.CategoryID
	}
	if selection.Kind == RowKindSession && selection.CategoryID != "" {
		return selection.CategoryID
	}
	if selection.Kind == RowKindSession && selection.Session != "" {
		if categoryID, ok := findSessionCategory(layout, selection.Session); ok {
			return categoryID
		}
	}
	return ""
}

func visibleCategorySessions(refs []SessionRef, showNumeric bool) []SessionRef {
	visible := make([]SessionRef, 0, len(refs))
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" || (!showNumeric && sessions.IsNumericName(name)) {
			continue
		}
		visible = append(visible, ref)
	}
	return visible
}

func SlotMap(refs []SessionRef, showNumeric bool) map[string]int {
	slots := map[string]int{}
	slot := 1
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" || (!showNumeric && sessions.IsNumericName(name)) {
			continue
		}
		if slot > 10 {
			break
		}
		slots[name] = slot
		slot++
	}
	return slots
}

// MoveSelection moves the selected layout item one step in the sign direction of delta.
// The sidebar currently exposes single-step J/K movement, so deltas larger than one
// are intentionally normalized instead of interpreted as multi-step movement.
func MoveSelection(layout Layout, selection Selection, delta int) Layout {
	return MoveSelectionVisible(layout, selection, delta, true)
}

func MoveSelectionVisible(layout Layout, selection Selection, delta int, showNumeric bool) Layout {
	delta = stepDelta(delta)
	if delta == 0 {
		return layout
	}
	switch selection.Kind {
	case RowKindCategory, RowKindSeparator, RowKindSpacer:
		return moveTopLevel(layout, selection.ItemID, delta)
	case RowKindSession:
		return moveSession(layout, selection.CategoryID, selection.Session, delta, showNumeric)
	default:
		return layout
	}
}

func stepDelta(delta int) int {
	if delta < 0 {
		return -1
	}
	if delta > 0 {
		return 1
	}
	return 0
}

func moveTopLevel(layout Layout, itemIDValue string, delta int) Layout {
	items := cloneItems(layout.Items)
	from := -1
	for i, item := range items {
		if itemID(item) == itemIDValue {
			from = i
			break
		}
	}
	if from < 0 {
		return layout
	}
	to := min(max(from+delta, 0), len(items)-1)
	if to == from {
		return layout
	}
	items[from], items[to] = items[to], items[from]
	return Layout{Items: items}
}

func moveSession(layout Layout, categoryID string, sessionName string, delta int, showNumeric bool) Layout {
	if categoryID == "" {
		resolved, ok := findSessionCategory(layout, sessionName)
		if !ok {
			return layout
		}
		categoryID = resolved
	}
	items := cloneItems(layout.Items)
	catIndexes := categoryIndexes(items)
	catPos := -1
	for i, itemIndex := range catIndexes {
		category := normalizedCategory(items[itemIndex].Category)
		items[itemIndex].ID = category.ID
		items[itemIndex].Category = category
		if category.ID == categoryID {
			catPos = i
			break
		}
	}
	if catPos < 0 {
		return layout
	}
	fromSession := sessionIndex(items[catIndexes[catPos]].Category.Sessions, sessionName)
	if fromSession < 0 {
		return layout
	}
	if delta < 0 {
		return moveSessionUp(items, catIndexes, catPos, fromSession, showNumeric)
	}
	return moveSessionDown(items, catIndexes, catPos, fromSession, showNumeric)
}

func moveSessionUp(items []LayoutItem, catIndexes []int, catPos int, fromSession int, showNumeric bool) Layout {
	catIndex := catIndexes[catPos]
	sessions := items[catIndex].Category.Sessions
	if previous := previousVisibleSessionIndex(sessions, fromSession, showNumeric); previous >= 0 {
		sessions[previous], sessions[fromSession] = sessions[fromSession], sessions[previous]
		items[catIndex].Category.Sessions = sessions
		return Layout{Items: items}
	}
	if catPos == 0 {
		return Layout{Items: items}
	}
	moving := items[catIndex].Category.Sessions[fromSession]
	items[catIndex].Category.Sessions = removeSessionAt(items[catIndex].Category.Sessions, fromSession)
	prevIndex := catIndexes[catPos-1]
	items[prevIndex].Category.Sessions = append(items[prevIndex].Category.Sessions, moving)
	return Layout{Items: items}
}

func moveSessionDown(items []LayoutItem, catIndexes []int, catPos int, fromSession int, showNumeric bool) Layout {
	catIndex := catIndexes[catPos]
	sessions := items[catIndex].Category.Sessions
	if next := nextVisibleSessionIndex(sessions, fromSession, showNumeric); next >= 0 {
		sessions[fromSession], sessions[next] = sessions[next], sessions[fromSession]
		items[catIndex].Category.Sessions = sessions
		return Layout{Items: items}
	}
	if catPos >= len(catIndexes)-1 {
		return Layout{Items: items}
	}
	moving := sessions[fromSession]
	items[catIndex].Category.Sessions = removeSessionAt(sessions, fromSession)
	nextIndex := catIndexes[catPos+1]
	items[nextIndex].Category.Sessions = append([]SessionRef{moving}, items[nextIndex].Category.Sessions...)
	return Layout{Items: items}
}

func previousVisibleSessionIndex(refs []SessionRef, from int, showNumeric bool) int {
	for i := from - 1; i >= 0; i-- {
		if showNumeric || !sessions.IsNumericName(refs[i].Name) {
			return i
		}
	}
	return -1
}

func nextVisibleSessionIndex(refs []SessionRef, from int, showNumeric bool) int {
	for i := from + 1; i < len(refs); i++ {
		if showNumeric || !sessions.IsNumericName(refs[i].Name) {
			return i
		}
	}
	return -1
}

func validLiveNames(names []string) []string {
	valid := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && sessions.ValidateName(name) == nil && !sessions.IsHiddenName(name) {
			valid = append(valid, name)
		}
	}
	return valid
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func normalizedCategory(category Category) Category {
	name := strings.TrimSpace(category.Name)
	if name == "" {
		name = DefaultCategoryName
	}
	id := normalizeID(category.ID, "category", name)
	if category.ID == "" && name == DefaultCategoryName {
		id = DefaultCategoryID
	}
	category.ID = id
	category.Name = name
	return category
}

func normalizeID(id string, kind string, label string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.NewReplacer(" ", "-", ":", "-", "/", "-", "\\", "-").Replace(label)
	if label == "" {
		label = "item"
	}
	return fmt.Sprintf("%s:%s", kind, label)
}

func itemID(item LayoutItem) string {
	if item.ID != "" {
		return item.ID
	}
	switch item.Kind {
	case ItemKindCategory:
		return item.Category.ID
	case ItemKindSeparator:
		return item.Separator.ID
	case ItemKindSpacer:
		return item.Spacer.ID
	default:
		return ""
	}
}

func sessionItemID(categoryID string, sessionName string) string {
	return categoryID + "/session:" + sessionName
}

func findSessionCategory(layout Layout, sessionName string) (string, bool) {
	for _, item := range layout.Items {
		if item.Kind != ItemKindCategory {
			continue
		}
		category := normalizedCategory(item.Category)
		for _, ref := range category.Sessions {
			if ref.Name == sessionName {
				return category.ID, true
			}
		}
	}
	return "", false
}

func cloneItems(items []LayoutItem) []LayoutItem {
	cloned := make([]LayoutItem, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.Kind == ItemKindCategory {
			cloned[i].Category.Sessions = append([]SessionRef(nil), item.Category.Sessions...)
		}
	}
	return cloned
}

func categoryIndexes(items []LayoutItem) []int {
	indexes := make([]int, 0, len(items))
	for i, item := range items {
		if item.Kind == ItemKindCategory {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func sessionIndex(refs []SessionRef, name string) int {
	for i, ref := range refs {
		if ref.Name == name {
			return i
		}
	}
	return -1
}

func removeSessionAt(refs []SessionRef, index int) []SessionRef {
	return append(append([]SessionRef(nil), refs[:index]...), refs[index+1:]...)
}
