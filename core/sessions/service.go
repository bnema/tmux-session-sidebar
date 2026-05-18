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

func MoveOrder(live []string, order []string, session string, delta int) []string {
	ordered := ApplyOrder(live, order)
	index := -1
	for i, name := range ordered {
		if name == session {
			index = i
			break
		}
	}
	if index < 0 {
		return ordered
	}
	target := min(max(index+delta, 0), len(ordered)-1)
	if target == index {
		return ordered
	}
	ordered[index], ordered[target] = ordered[target], ordered[index]
	return ordered
}

func VisibleNames(names []string, showNumeric bool) []string {
	visible := make([]string, 0, len(names))
	for _, name := range names {
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
