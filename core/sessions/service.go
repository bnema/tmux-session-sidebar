package sessions

import (
	"regexp"

	coreerrors "github.com/bnema/tmux-session-sidebar/core/errors"
)

var validName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type View struct {
	SessionID string
	Name      string
	Visible   bool
	Current   bool
}

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return coreerrors.ErrInvalidSessionName
	}
	return nil
}

func IsHiddenName(name string) bool {
	return len(name) >= 2 && name[0:2] == "__"
}

func IsNumericName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func IsPersistableName(name string) bool {
	return name != "" && ValidateName(name) == nil && !IsNumericName(name) && !IsHiddenName(name)
}

func ApplyOrder(live []string, order []string) []string {
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}
	used := make(map[string]bool, len(live))
	ordered := make([]string, 0, len(live))
	for _, name := range order {
		if liveSet[name] && !used[name] {
			ordered = append(ordered, name)
			used[name] = true
		}
	}
	for _, name := range live {
		if !used[name] {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

func ApplyPinnedPositions(anchor []string, order []string, pinned []string) []string {
	if len(anchor) == 0 || len(order) == 0 || len(pinned) == 0 {
		return append([]string(nil), order...)
	}
	pinnedSet := make(map[string]bool, len(pinned))
	for _, name := range pinned {
		pinnedSet[name] = true
	}
	orderSet := make(map[string]bool, len(order))
	for _, name := range order {
		orderSet[name] = true
	}
	result := make([]string, len(order))
	reserved := make(map[string]bool, len(pinnedSet))
	for index, name := range anchor {
		if index >= len(result) {
			break
		}
		if pinnedSet[name] && orderSet[name] {
			result[index] = name
			reserved[name] = true
		}
	}
	fillIndex := 0
	for _, name := range order {
		if reserved[name] {
			continue
		}
		for fillIndex < len(result) && result[fillIndex] != "" {
			fillIndex++
		}
		if fillIndex >= len(result) {
			break
		}
		result[fillIndex] = name
	}
	return result
}

func ReconcilePinned(pinned []string, live []string) []string {
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}
	used := make(map[string]bool, len(pinned))
	reconciled := make([]string, 0, len(pinned))
	for _, name := range pinned {
		if liveSet[name] && !used[name] {
			reconciled = append(reconciled, name)
			used[name] = true
		}
	}
	return reconciled
}

func ReconcileNamedStrings(values map[string]string, live []string) map[string]string {
	if len(values) == 0 {
		return values
	}
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}
	for name := range values {
		if !liveSet[name] {
			delete(values, name)
		}
	}
	return values
}

func TogglePinned(pinned []string, session string) ([]string, bool) {
	for i, name := range pinned {
		if name == session {
			next := append([]string(nil), pinned[:i]...)
			next = append(next, pinned[i+1:]...)
			return next, false
		}
	}
	next := append([]string(nil), pinned...)
	next = append(next, session)
	return next, true
}

func MoveOrder(live []string, order []string, session string, delta int) []string {
	ordered := ApplyOrder(live, order)
	return moveOrderAtIndices(ordered, allIndices(ordered), session, delta)
}

func MoveVisibleOrder(live []string, order []string, session string, delta int, showNumeric bool) []string {
	ordered := ApplyOrder(live, order)
	indices := make([]int, 0, len(ordered))
	for i, name := range ordered {
		if ValidateName(name) != nil {
			continue
		}
		if IsHiddenName(name) {
			continue
		}
		if IsNumericName(name) && !showNumeric {
			continue
		}
		indices = append(indices, i)
	}
	return moveOrderAtIndices(ordered, indices, session, delta)
}

func moveOrderAtIndices(ordered []string, indices []int, session string, delta int) []string {
	visibleIndex := -1
	for i, orderedIndex := range indices {
		if ordered[orderedIndex] == session {
			visibleIndex = i
			break
		}
	}
	if visibleIndex < 0 {
		return ordered
	}
	target := min(max(visibleIndex+delta, 0), len(indices)-1)
	if target == visibleIndex {
		return ordered
	}
	fromIndex := indices[visibleIndex]
	toIndex := indices[target]
	ordered[fromIndex], ordered[toIndex] = ordered[toIndex], ordered[fromIndex]
	return ordered
}

func allIndices(values []string) []int {
	indices := make([]int, len(values))
	for i := range values {
		indices[i] = i
	}
	return indices
}

func VisibleNames(names []string, showNumeric bool) []string {
	visible := make([]string, 0, len(names))
	for _, name := range names {
		if ValidateName(name) != nil {
			continue
		}
		if IsHiddenName(name) {
			continue
		}
		if IsNumericName(name) && !showNumeric {
			continue
		}
		visible = append(visible, name)
	}
	return visible
}

func FilterVisible(all []View, showNumeric bool) []View {
	visible := make([]View, 0, len(all))
	for _, session := range all {
		if !session.Visible {
			continue
		}
		if ValidateName(session.Name) != nil {
			continue
		}
		if IsHiddenName(session.Name) {
			continue
		}
		if IsNumericName(session.Name) && !showNumeric {
			continue
		}
		visible = append(visible, session)
	}
	return visible
}
