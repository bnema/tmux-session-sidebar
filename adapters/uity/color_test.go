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

func TestInactiveSessionColorGradient(t *testing.T) {
	tests := map[string]struct {
		intensity float64
		wantHex   string
		wantANSI  string
	}{
		"dark endpoint stays current inactive gray": {intensity: 0, wantHex: "#4b5563", wantANSI: "\033[38;2;75;85;99m"},
		"midpoint lifts toward cool light gray":     {intensity: 0.5, wantHex: "#8c9198", wantANSI: "\033[38;2;140;145;152m"},
		"fresh endpoint reaches light gray":         {intensity: 1, wantHex: "#cccccc", wantANSI: "\033[38;2;204;204;204m"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := inactiveSessionColor(tt.intensity); got != tt.wantHex {
				t.Fatalf("inactiveSessionColor(%v) = %q, want %q", tt.intensity, got, tt.wantHex)
			}
			if got := inactiveSessionRGBForIntensity(tt.intensity).ANSI(); got != tt.wantANSI {
				t.Fatalf("inactiveSessionRGBForIntensity(%v).ANSI() = %q, want %q", tt.intensity, got, tt.wantANSI)
			}
		})
	}
}

func TestInactiveMetadataColorGradientStaysDarkerThanSessionGradient(t *testing.T) {
	tests := map[string]struct {
		intensity float64
		wantHex   string
		wantANSI  string
	}{
		"dark endpoint stays current inactive metadata gray":           {intensity: 0, wantHex: "#374151", wantANSI: "\033[38;2;55;65;81m"},
		"midpoint stays slightly darker than session midpoint":         {intensity: 0.5, wantHex: "#787d86", wantANSI: "\033[38;2;120;125;134m"},
		"fresh endpoint stays slightly darker than session light gray": {intensity: 1, wantHex: "#b8b8ba", wantANSI: "\033[38;2;184;184;186m"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := inactiveMetadataColor(tt.intensity); got != tt.wantHex {
				t.Fatalf("inactiveMetadataColor(%v) = %q, want %q", tt.intensity, got, tt.wantHex)
			}
			if got := inactiveMetadataRGBForIntensity(tt.intensity).ANSI(); got != tt.wantANSI {
				t.Fatalf("inactiveMetadataRGBForIntensity(%v).ANSI() = %q, want %q", tt.intensity, got, tt.wantANSI)
			}
		})
	}
}
