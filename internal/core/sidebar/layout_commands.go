package sidebar

import (
	"fmt"
	"strings"
)

func AddCategory(layout Layout, live []string, order []string, name string) (Layout, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return layout, fmt.Errorf("create sidebar category: name is required")
	}
	layout = EnsureLayout(layout, live, order)
	layout.Items = append(layout.Items, CategoryItem(uniqueLayoutID("category", layout), name, false, nil))
	return layout, nil
}

func AssignSessionCategory(layout Layout, live []string, order []string, sessionName string, categoryID string) (Layout, bool) {
	sessionName = strings.TrimSpace(sessionName)
	categoryID = strings.TrimSpace(categoryID)
	if sessionName == "" || categoryID == "" {
		return layout, false
	}
	layout = EnsureLayout(layout, live, order)
	items := cloneItems(layout.Items)
	foundTarget := false
	for itemIndex := range items {
		if items[itemIndex].Kind != ItemKindCategory {
			continue
		}
		category := &items[itemIndex].Category
		kept := category.Sessions[:0]
		for _, ref := range category.Sessions {
			if ref.Name != sessionName {
				kept = append(kept, ref)
			}
		}
		category.Sessions = kept
		if category.ID == categoryID {
			foundTarget = true
			category.Sessions = append(category.Sessions, SessionRef{Name: sessionName})
		}
	}
	if !foundTarget || layoutItemsEqual(layout.Items, items) {
		return layout, false
	}
	return Layout{Items: items}, true
}

func SetCategoryColor(layout Layout, live []string, order []string, categoryID string, color string) (Layout, bool, error) {
	categoryID = strings.TrimSpace(categoryID)
	color = strings.TrimSpace(color)
	if categoryID == "" {
		return layout, false, nil
	}
	layout = EnsureLayout(layout, live, order)
	items := cloneItems(layout.Items)
	for i := range items {
		if items[i].Kind == ItemKindCategory && items[i].Category.ID == categoryID {
			if items[i].Category.Color == color {
				return layout, false, nil
			}
			items[i].Category.Color = color
			return Layout{Items: items}, true, nil
		}
	}
	if color == "" {
		return layout, false, nil
	}
	return layout, false, fmt.Errorf("color sidebar category: category %q not found", categoryID)
}

func SetCategoryCollapsed(layout Layout, live []string, order []string, categoryID string, collapsed bool) (Layout, bool) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return layout, false
	}
	layout = EnsureLayout(layout, live, order)
	items := cloneItems(layout.Items)
	for i := range items {
		if items[i].Kind == ItemKindCategory && items[i].Category.ID == categoryID {
			if items[i].Category.Collapsed == collapsed {
				return layout, false
			}
			items[i].Category.Collapsed = collapsed
			return Layout{Items: items}, true
		}
	}
	return layout, false
}

func SetCategorySessionsExpanded(layout Layout, live []string, order []string, categoryID string, expanded bool) (Layout, bool) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return layout, false
	}
	layout = EnsureLayout(layout, live, order)
	items := cloneItems(layout.Items)
	for i := range items {
		if items[i].Kind == ItemKindCategory && items[i].Category.ID == categoryID {
			if items[i].Category.SessionsExpanded == expanded {
				return layout, false
			}
			items[i].Category.SessionsExpanded = expanded
			return Layout{Items: items}, true
		}
	}
	return layout, false
}

func RenameCategory(layout Layout, live []string, order []string, categoryID string, name string) (Layout, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return layout, fmt.Errorf("rename sidebar category: name is required")
	}
	layout = EnsureLayout(layout, live, order)
	items := cloneItems(layout.Items)
	for i := range items {
		if items[i].Kind == ItemKindCategory && items[i].Category.ID == categoryID {
			items[i].Category.Name = name
			return Layout{Items: items}, nil
		}
	}
	return layout, fmt.Errorf("rename sidebar category: category %q not found", categoryID)
}

func AddSpacer(layout Layout, live []string, order []string) Layout {
	layout = EnsureLayout(layout, live, order)
	layout.Items = append(layout.Items, SpacerItem(uniqueLayoutID("spacer", layout)))
	return layout
}

func AddSeparator(layout Layout, live []string, order []string) Layout {
	layout = EnsureLayout(layout, live, order)
	layout.Items = append(layout.Items, SeparatorItem(uniqueLayoutID("separator", layout)))
	return layout
}

func MoveSelectionUseCase(layout Layout, live []string, order []string, selection Selection, delta int, showNumeric bool) Layout {
	layout = EnsureLayout(layout, live, order)
	return MoveSelectionVisible(layout, selection, delta, showNumeric)
}

func DeleteSelection(layout Layout, live []string, order []string, selection Selection) Layout {
	layout = EnsureLayout(layout, live, order)
	items := make([]LayoutItem, 0, len(layout.Items))
	for _, item := range layout.Items {
		if layoutItemMatchesSelection(item, selection) {
			continue
		}
		items = append(items, item)
	}
	return EnsureLayout(Layout{Items: items}, live, order)
}

func layoutItemMatchesSelection(item LayoutItem, selection Selection) bool {
	switch selection.Kind {
	case RowKindCategory:
		return item.Kind == ItemKindCategory && item.Category.ID == selection.CategoryID
	case RowKindSeparator, RowKindSpacer:
		return itemID(item) == selection.ItemID
	default:
		return false
	}
}

func uniqueLayoutID(prefix string, layout Layout) string {
	used := map[string]bool{}
	for _, item := range layout.Items {
		used[itemID(item)] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("%s:%d", prefix, i)
		if !used[id] {
			return id
		}
	}
}

func layoutItemsEqual(a []LayoutItem, b []LayoutItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Kind != b[i].Kind || a[i].Separator != b[i].Separator || a[i].Spacer != b[i].Spacer {
			return false
		}
		if a[i].Category.ID != b[i].Category.ID || a[i].Category.Name != b[i].Category.Name || a[i].Category.Color != b[i].Category.Color || a[i].Category.Collapsed != b[i].Category.Collapsed || a[i].Category.SessionsExpanded != b[i].Category.SessionsExpanded {
			return false
		}
		if len(a[i].Category.Sessions) != len(b[i].Category.Sessions) {
			return false
		}
		for j := range a[i].Category.Sessions {
			if a[i].Category.Sessions[j] != b[i].Category.Sessions[j] {
				return false
			}
		}
	}
	return true
}
