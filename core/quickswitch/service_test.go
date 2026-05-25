package quickswitch

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func TestBuildSlotMapSkipsNumericSessions(t *testing.T) {
	tests := []struct {
		name     string
		sessions []sessions.View
		want     map[int]string
	}{
		{
			name: "skips numeric",
			sessions: []sessions.View{
				{SessionID: "123", Name: "123", Visible: true},
				{SessionID: "alpha", Name: "alpha", Visible: true},
				{SessionID: "beta", Name: "beta", Visible: true},
			},
			want: map[int]string{1: "alpha", 2: "beta"},
		},
		{
			name: "skips hidden",
			sessions: []sessions.View{
				{SessionID: "alpha", Name: "alpha", Visible: false},
				{SessionID: "beta", Name: "beta", Visible: true},
			},
			want: map[int]string{1: "beta"},
		},
		{
			name: "caps at ten",
			sessions: []sessions.View{
				{Name: "a", Visible: true}, {Name: "b", Visible: true}, {Name: "c", Visible: true},
				{Name: "d", Visible: true}, {Name: "e", Visible: true}, {Name: "f", Visible: true},
				{Name: "g", Visible: true}, {Name: "h", Visible: true}, {Name: "i", Visible: true},
				{Name: "j", Visible: true}, {Name: "k", Visible: true},
			},
			want: map[int]string{1: "a", 2: "b", 3: "c", 4: "d", 5: "e", 6: "f", 7: "g", 8: "h", 9: "i", 10: "j"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSlotMap(tt.sessions)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for slot, want := range tt.want {
				if got[slot] != want {
					t.Fatalf("slot %d = %q, want %q", slot, got[slot], want)
				}
			}
		})
	}
}

func TestBadgeForSlot(t *testing.T) {
	tests := []struct {
		name string
		slot int
		want string
	}{
		{name: "one", slot: 1, want: "[1]"},
		{name: "nine", slot: 9, want: "[9]"},
		{name: "ten displays as ten", slot: 10, want: "[10]"},
		{name: "eleven displays as label only", slot: 11, want: "[11]"},
		{name: "zero invalid", slot: 0, want: ""},
		{name: "negative invalid", slot: -1, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BadgeForSlot(tt.slot); got != tt.want {
				t.Fatalf("BadgeForSlot(%d) = %q, want %q", tt.slot, got, tt.want)
			}
		})
	}
}
