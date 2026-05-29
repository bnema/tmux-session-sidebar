package uity

import (
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func TestRenderRows(t *testing.T) {
	tests := []struct {
		name       string
		rows       []Row
		capability Capability
		wantParts  []string
		wantExact  string
	}{
		{name: "empty rows", rows: nil, capability: CapabilityPlain, wantExact: ""},
		{name: "plain current with badge", rows: []Row{{Session: sessions.View{Name: "alpha", Current: true}, Slot: 1}}, capability: CapabilityPlain, wantParts: []string{"* [1] alpha\n"}},
		{name: "rgb heat style", rows: []Row{{Session: sessions.View{Name: "beta"}, Bucket: heat.BucketCurrent, HeatIntensity: 1}}, capability: CapabilityRGB, wantParts: []string{"\033[38;2;240;253;244m", " beta\033[0m\n"}},
		{name: "strips ansi from names", rows: []Row{{Session: sessions.View{Name: "\033[31mred\033[0m"}}}, capability: CapabilityPlain, wantExact: "  red\n"},
		{name: "empty name", rows: []Row{{Session: sessions.View{Name: ""}}}, capability: CapabilityPlain, wantExact: "  \n"},
		{name: "multi row order", rows: []Row{{Session: sessions.View{Name: "alpha"}}, {Session: sessions.View{Name: "beta"}}}, capability: CapabilityPlain, wantExact: "  alpha\n  beta\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.rows, tt.capability)
			if tt.wantExact != "" && got != tt.wantExact {
				t.Fatalf("Render() = %q, want %q", got, tt.wantExact)
			}
			for _, want := range tt.wantParts {
				if !strings.Contains(got, want) {
					t.Fatalf("Render() = %q, expected to contain %q", got, want)
				}
			}
		})
	}
}

func TestHeatStyle(t *testing.T) {
	tests := []struct {
		name       string
		bucket     heat.Bucket
		intensity  float64
		capability Capability
		want       string
	}{
		{name: "rgb current hot", bucket: heat.BucketCurrent, intensity: 1, capability: CapabilityRGB, want: "\033[38;2;240;253;244m"},
		{name: "rgb current midpoint", bucket: heat.BucketCurrent, intensity: 0.5, capability: CapabilityRGB, want: "\033[38;2;131;177;148m"},
		{name: "rgb current cool", bucket: heat.BucketCurrent, intensity: 0, capability: CapabilityRGB, want: "\033[38;2;22;101;52m"},
		{name: "256 stale", bucket: heat.BucketStale, intensity: 1, capability: Capability256, want: "\033[2;38;5;244m"},
		{name: "basic current uses highlight", bucket: heat.BucketCurrent, intensity: 1, capability: CapabilityBasic, want: "\033[32m"},
		{name: "plain current empty", bucket: heat.BucketCurrent, intensity: 1, capability: CapabilityPlain, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HeatStyle(tt.bucket, tt.intensity, tt.capability); got != tt.want {
				t.Fatalf("HeatStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}
