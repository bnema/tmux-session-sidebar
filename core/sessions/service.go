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

func FilterVisible(all []View, showNumeric bool) []View {
	visible := make([]View, 0, len(all))
	for _, session := range all {
		if !session.Visible {
			continue
		}
		if IsNumericName(session.Name) && !showNumeric {
			continue
		}
		visible = append(visible, session)
	}
	return visible
}
