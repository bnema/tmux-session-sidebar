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
		{name: "rgb heat style", rows: []Row{{Session: sessions.View{Name: "beta"}, Bucket: heat.BucketHot}}, capability: CapabilityRGB, wantParts: []string{"\033[38;2;220;252;231m", " beta\033[0m\n"}},
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
		capability Capability
		wantEmpty  bool
	}{
		{name: "rgb current", bucket: heat.BucketCurrent, capability: CapabilityRGB, wantEmpty: false},
		{name: "256 stale", bucket: heat.BucketStale, capability: Capability256, wantEmpty: false},
		{name: "basic cool uses highlight", bucket: heat.BucketCool, capability: CapabilityBasic, wantEmpty: false},
		{name: "plain hot empty", bucket: heat.BucketHot, capability: CapabilityPlain, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HeatStyle(tt.bucket, tt.capability)
			if (got == "") != tt.wantEmpty {
				t.Fatalf("HeatStyle() = %q, wantEmpty %v", got, tt.wantEmpty)
			}
		})
	}
}
