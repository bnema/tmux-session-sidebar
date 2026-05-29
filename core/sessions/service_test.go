package sessions

import (
	"reflect"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "simple", in: "alpha", wantErr: false},
		{name: "mixed allowed", in: "A_1-z", wantErr: false},
		{name: "numeric allowed", in: "123", wantErr: false},
		{name: "empty", in: "", wantErr: true},
		{name: "space", in: "bad name", wantErr: true},
		{name: "slash", in: "bad/name", wantErr: true},
		{name: "colon", in: "bad:name", wantErr: true},
		{name: "dot", in: "bad.name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.in)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("ValidateName(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestIsNumericName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "numeric", in: "123", want: true},
		{name: "zero", in: "0", want: true},
		{name: "mixed", in: "123a", want: false},
		{name: "empty", in: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNumericName(tt.in); got != tt.want {
				t.Fatalf("IsNumericName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyOrder(t *testing.T) {
	tests := []struct {
		name  string
		live  []string
		order []string
		want  []string
	}{
		{name: "saved order first and new sessions appended", live: []string{"alpha", "beta", "gamma", "delta"}, order: []string{"gamma", "missing", "alpha"}, want: []string{"gamma", "alpha", "beta", "delta"}},
		{name: "duplicates in saved order are ignored", live: []string{"alpha", "beta", "gamma"}, order: []string{"beta", "beta", "alpha"}, want: []string{"beta", "alpha", "gamma"}},
		{name: "nil saved order keeps live order", live: []string{"alpha", "beta"}, order: nil, want: []string{"alpha", "beta"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyOrder(tt.live, tt.order)
			if len(got) != len(tt.want) {
				t.Fatalf("ApplyOrder() = %#v, want %#v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Fatalf("ApplyOrder()[%d] = %q, want %q (full: %#v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestApplyPinnedPositions(t *testing.T) {
	tests := []struct {
		name   string
		anchor []string
		order  []string
		pinned []string
		want   []string
	}{
		{
			name:   "keeps pinned sessions at anchor positions and fills around them",
			anchor: []string{"alpha", "beta", "gamma", "delta"},
			order:  []string{"delta", "gamma", "beta", "alpha"},
			pinned: []string{"beta"},
			want:   []string{"delta", "beta", "gamma", "alpha"},
		},
		{
			name:   "multiple pinned sessions keep their anchor positions",
			anchor: []string{"alpha", "beta", "gamma", "delta"},
			order:  []string{"delta", "gamma", "beta", "alpha"},
			pinned: []string{"alpha", "gamma"},
			want:   []string{"alpha", "delta", "gamma", "beta"},
		},
		{
			name:   "missing pinned sessions are ignored",
			anchor: []string{"alpha", "beta", "gamma"},
			order:  []string{"gamma", "beta", "alpha"},
			pinned: []string{"missing", "beta"},
			want:   []string{"gamma", "beta", "alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyPinnedPositions(tt.anchor, tt.order, tt.pinned)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ApplyPinnedPositions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReconcilePinned(t *testing.T) {
	got := ReconcilePinned([]string{"beta", "missing", "alpha", "beta"}, []string{"alpha", "beta", "gamma"})
	want := []string{"beta", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReconcilePinned() = %#v, want %#v", got, want)
	}
}

func TestTogglePinned(t *testing.T) {
	pinned, added := TogglePinned([]string{"alpha"}, "beta")
	if !added || !reflect.DeepEqual(pinned, []string{"alpha", "beta"}) {
		t.Fatalf("TogglePinned add = %#v added=%v", pinned, added)
	}
	pinned, added = TogglePinned(pinned, "alpha")
	if added || !reflect.DeepEqual(pinned, []string{"beta"}) {
		t.Fatalf("TogglePinned remove = %#v added=%v", pinned, added)
	}
}

func TestMoveOrder(t *testing.T) {
	tests := []struct {
		name    string
		live    []string
		order   []string
		session string
		delta   int
		want    []string
	}{
		{name: "moves selected down after applying saved order", live: []string{"alpha", "beta", "gamma"}, order: []string{"gamma", "alpha", "beta"}, session: "alpha", delta: 1, want: []string{"gamma", "beta", "alpha"}},
		{name: "clamps first session moving up", live: []string{"alpha", "beta", "gamma"}, order: nil, session: "alpha", delta: -1, want: []string{"alpha", "beta", "gamma"}},
		{name: "clamps last session moving down", live: []string{"alpha", "beta", "gamma"}, order: nil, session: "gamma", delta: 1, want: []string{"alpha", "beta", "gamma"}},
		{name: "missing selected session keeps applied order", live: []string{"alpha", "beta"}, order: []string{"beta", "alpha"}, session: "missing", delta: 1, want: []string{"beta", "alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MoveOrder(tt.live, tt.order, tt.session, tt.delta)
			if len(got) != len(tt.want) {
				t.Fatalf("MoveOrder() = %#v, want %#v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Fatalf("MoveOrder()[%d] = %q, want %q (full: %#v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestMoveVisibleOrder(t *testing.T) {
	tests := []struct {
		name        string
		live        []string
		order       []string
		session     string
		delta       int
		showNumeric bool
		want        []string
	}{
		{name: "jumps over hidden numeric when moving to first visible", live: []string{"1", "alpha", "beta"}, order: nil, session: "alpha", delta: -1, want: []string{"1", "alpha", "beta"}},
		{name: "moves visible session above previous visible despite hidden first", live: []string{"1", "alpha", "beta"}, order: nil, session: "beta", delta: -1, want: []string{"1", "beta", "alpha"}},
		{name: "includes numeric when shown", live: []string{"1", "alpha", "beta"}, order: nil, session: "alpha", delta: -1, showNumeric: true, want: []string{"alpha", "1", "beta"}},
		{name: "hidden sessions are always skipped", live: []string{"__scratch", "alpha", "beta"}, order: nil, session: "beta", delta: -1, showNumeric: true, want: []string{"__scratch", "beta", "alpha"}},
		{name: "invalid sessions are skipped", live: []string{"alpha", "bad name", "beta"}, order: nil, session: "beta", delta: -1, showNumeric: true, want: []string{"beta", "bad name", "alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MoveVisibleOrder(tt.live, tt.order, tt.session, tt.delta, tt.showNumeric)
			if len(got) != len(tt.want) {
				t.Fatalf("MoveVisibleOrder() = %#v, want %#v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Fatalf("MoveVisibleOrder()[%d] = %q, want %q (full: %#v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestIsHiddenName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "double underscore hidden", in: "__scratch", want: true},
		{name: "single underscore visible", in: "_scratch", want: false},
		{name: "normal visible", in: "alpha", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHiddenName(tt.in); got != tt.want {
				t.Fatalf("IsHiddenName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestVisibleNames(t *testing.T) {
	tests := []struct {
		name        string
		names       []string
		showNumeric bool
		want        []string
	}{
		{name: "hides numeric internal and invalid by default", names: []string{"alpha", "123", "__scratch", "bad name", "beta"}, showNumeric: false, want: []string{"alpha", "beta"}},
		{name: "shows numeric but still hides invalid when requested", names: []string{"alpha", "123", "__scratch", "bad name"}, showNumeric: true, want: []string{"alpha", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VisibleNames(tt.names, tt.showNumeric)
			if len(got) != len(tt.want) {
				t.Fatalf("VisibleNames() = %#v, want %#v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Fatalf("VisibleNames()[%d] = %q, want %q (full: %#v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestFilterVisible(t *testing.T) {
	all := []View{
		{SessionID: "1", Name: "alpha", Visible: true},
		{SessionID: "2", Name: "123", Visible: true},
		{SessionID: "3", Name: "hidden", Visible: false},
		{SessionID: "4", Name: "bad name", Visible: true},
	}
	tests := []struct {
		name        string
		showNumeric bool
		wantNames   []string
	}{
		{name: "hide numeric", showNumeric: false, wantNames: []string{"alpha"}},
		{name: "show numeric", showNumeric: true, wantNames: []string{"alpha", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterVisible(all, tt.showNumeric)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.wantNames), got)
			}
			for i, want := range tt.wantNames {
				if got[i].Name != want {
					t.Fatalf("name[%d] = %q, want %q", i, got[i].Name, want)
				}
			}
		})
	}
}
