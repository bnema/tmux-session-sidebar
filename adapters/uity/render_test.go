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
	}{
		{name: "plain current with badge", rows: []Row{{Session: sessions.View{Name: "alpha", Current: true}, Slot: 1}}, capability: CapabilityPlain, wantParts: []string{"● [1] alpha\n"}},
		{name: "rgb heat style", rows: []Row{{Session: sessions.View{Name: "beta"}, Bucket: heat.BucketHot}}, capability: CapabilityRGB, wantParts: []string{"\033[38;2;122;232;122m", " beta\033[0m\n"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.rows, tt.capability)
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
		{name: "basic cool empty", bucket: heat.BucketCool, capability: CapabilityBasic, wantEmpty: true},
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
