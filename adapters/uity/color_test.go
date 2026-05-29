package uity

import "testing"

func TestHeatColorGradient(t *testing.T) {
	tests := map[string]struct {
		intensity float64
		wantHex   string
		wantANSI  string
	}{
		"clamps below zero to cool green": {intensity: -0.5, wantHex: "#166534", wantANSI: "\033[38;2;22;101;52m"},
		"zero is cool green":              {intensity: 0, wantHex: "#166534", wantANSI: "\033[38;2;22;101;52m"},
		"midpoint blends greens":          {intensity: 0.5, wantHex: "#83b194", wantANSI: "\033[38;2;131;177;148m"},
		"one is hot light green":          {intensity: 1, wantHex: "#f0fdf4", wantANSI: "\033[38;2;240;253;244m"},
		"clamps above one to hot":         {intensity: 2, wantHex: "#f0fdf4", wantANSI: "\033[38;2;240;253;244m"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := heatColor(tt.intensity); got != tt.wantHex {
				t.Fatalf("heatColor(%v) = %q, want %q", tt.intensity, got, tt.wantHex)
			}
			if got := heatRGB(tt.intensity).ANSI(); got != tt.wantANSI {
				t.Fatalf("heatRGB(%v).ANSI() = %q, want %q", tt.intensity, got, tt.wantANSI)
			}
		})
	}
}

func TestInactiveSessionRGBDimANSI(t *testing.T) {
	if got := inactiveSessionRGB.DimANSI(); got != "\033[2;38;2;75;85;99m" {
		t.Fatalf("DimANSI() = %q, want inactive gray dim ANSI", got)
	}
}
