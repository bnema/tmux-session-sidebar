package uity

import tea "charm.land/bubbletea/v2"

type Mode string

const (
	ModeBrowse         Mode = "browse"
	ModeSearch         Mode = "search"
	ModeProject        Mode = "project"
	ModeCreate         Mode = "create"
	ModeCreateNamed    Mode = "create-named"
	ModeCreateCategory Mode = "create-category"
	ModeRenameCategory Mode = "rename-category"
	ModeConfirmKill    Mode = "confirm-kill"
	ModeConfirmDelete  Mode = "confirm-delete"
	ModePinColor       Mode = "pin-color"
)

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

func navigationKeyDelta(msg tea.KeyPressMsg) (int, bool) {
	key := msg.Key()
	if key.Mod != 0 && !key.Mod.Contains(tea.ModAlt) {
		return 0, false
	}
	switch key.Code {
	case tea.KeyDown:
		return 1, true
	case tea.KeyUp:
		return -1, true
	}
	switch {
	case key.Text == "j" || key.Code == 'j':
		return 1, true
	case key.Text == "k" || key.Code == 'k':
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
