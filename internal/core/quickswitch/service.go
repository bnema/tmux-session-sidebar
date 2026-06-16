package quickswitch

import (
	"fmt"

	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
)

func BuildSlotMap(visible []sessions.View) map[int]string {
	slots := make(map[int]string)
	nextSlot := 1
	for _, session := range visible {
		if !session.Visible || sessions.IsNumericName(session.Name) {
			continue
		}
		slots[nextSlot] = session.Name
		nextSlot++
	}
	return slots
}

func BadgeForSlot(slot int) string {
	if slot <= 0 {
		return ""
	}
	return fmt.Sprintf("[%d]", slot)
}
