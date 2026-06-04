package uity

import (
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func TestRenderScreenIncludesControlsMarkersBadgesAndColors(t *testing.T) {
	tests := []struct {
		name      string
		screen    Screen
		contains  []string
		notInText []string
	}{
		{
			name: "browse screen",
			screen: Screen{Mode: ModeBrowse, Capability: CapabilityRGB, Rows: []Row{
				{Session: sessions.View{Name: "alpha", Current: true}, Bucket: heat.BucketCurrent, HeatIntensity: 1, Slot: 1},
				{Session: sessions.View{Name: "beta"}, Bucket: heat.BucketCurrent, HeatIntensity: 1, Slot: 2},
			}},
			contains: []string{"sessions browse", "↵ switch", "n project", "a adhoc", "u update", "r rename", "x kill", "h nums", "? help", "* [1] alpha", "[2] beta", "\033[38;2;240;253;244m"},
		},
		{
			name:      "search screen with sanitized filter",
			screen:    Screen{Mode: ModeSearch, Filter: "\033[31mbeta", Capability: CapabilityPlain, Rows: []Row{{Session: sessions.View{Name: "beta"}}}},
			contains:  []string{"sessions search", "filter:beta", "Esc close"},
			notInText: []string{"\033[31m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderScreen(tt.screen)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("RenderScreen() missing %q in %q", want, got)
				}
			}
			for _, unwanted := range tt.notInText {
				if strings.Contains(got, unwanted) {
					t.Fatalf("RenderScreen() contains unwanted %q in %q", unwanted, got)
				}
			}
		})
	}
}
