package sidebar

import "github.com/bnema/tmux-session-sidebar/core/sessions"

func EnterSearch(state State) State {
	state.Mode = ModeSearch
	return state
}

func UpdateFilter(state State, filter string) State {
	state.Filter = filter
	return state
}

func ApplySearch(state State) State {
	state.Mode = ModeBrowse
	return state
}

func CancelSearch(state State) State {
	state.Mode = ModeBrowse
	state.Filter = ""
	return state
}

func ToggleNumericSessions(state State) State {
	state.ShowNumericSessions = !state.ShowNumericSessions
	return state
}

func MoveSelectionUp(state State, visible []sessions.View) State {
	return moveSelection(state, visible, -1)
}

func MoveSelectionDown(state State, visible []sessions.View) State {
	return moveSelection(state, visible, 1)
}

func moveSelection(state State, visible []sessions.View, delta int) State {
	if len(visible) == 0 {
		state.SelectionSessionID = ""
		return state
	}
	current := 0
	for i, session := range visible {
		if session.SessionID == state.SelectionSessionID {
			current = i
			break
		}
	}
	next := (current + delta + len(visible)) % len(visible)
	state.SelectionSessionID = visible[next].SessionID
	return state
}
