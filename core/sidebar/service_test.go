package sidebar

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func TestSidebarSearchTransitions(t *testing.T) {
	tests := []struct {
		name string
		from State
		act  func(State) State
		want State
	}{
		{name: "enter search", from: State{Mode: ModeBrowse}, act: EnterSearch, want: State{Mode: ModeSearch}},
		{name: "update filter", from: State{Mode: ModeSearch}, act: func(s State) State { return UpdateFilter(s, "beta") }, want: State{Mode: ModeSearch, Filter: "beta"}},
		{name: "apply search keeps filter", from: State{Mode: ModeSearch, Filter: "beta"}, act: ApplySearch, want: State{Mode: ModeBrowse, Filter: "beta"}},
		{name: "cancel search clears filter", from: State{Mode: ModeSearch, Filter: "beta"}, act: CancelSearch, want: State{Mode: ModeBrowse, Filter: ""}},
		{name: "toggle numeric", from: State{}, act: ToggleNumericSessions, want: State{ShowNumericSessions: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.act(tt.from); got != tt.want {
				t.Fatalf("state = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSidebarSelectionMovement(t *testing.T) {
	visible := []sessions.View{{SessionID: "a"}, {SessionID: "b"}, {SessionID: "c"}}
	tests := []struct {
		name string
		from State
		act  func(State, []sessions.View) State
		want string
	}{
		{name: "down from first", from: State{SelectionSessionID: "a"}, act: MoveSelectionDown, want: "b"},
		{name: "down wraps", from: State{SelectionSessionID: "c"}, act: MoveSelectionDown, want: "a"},
		{name: "up from middle", from: State{SelectionSessionID: "b"}, act: MoveSelectionUp, want: "a"},
		{name: "up wraps", from: State{SelectionSessionID: "a"}, act: MoveSelectionUp, want: "c"},
		{name: "missing selection starts at second for down", from: State{SelectionSessionID: "missing"}, act: MoveSelectionDown, want: "b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.act(tt.from, visible)
			if got.SelectionSessionID != tt.want {
				t.Fatalf("selection = %q, want %q", got.SelectionSessionID, tt.want)
			}
		})
	}
}

func TestSidebarSelectionEmptyList(t *testing.T) {
	tests := []struct {
		name string
		from State
		act  func(State, []sessions.View) State
		want string
	}{
		{name: "down clears", from: State{SelectionSessionID: "a"}, act: MoveSelectionDown, want: ""},
		{name: "up clears", from: State{SelectionSessionID: "a"}, act: MoveSelectionUp, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.act(tt.from, nil)
			if got.SelectionSessionID != tt.want {
				t.Fatalf("selection = %q, want %q", got.SelectionSessionID, tt.want)
			}
		})
	}
}
