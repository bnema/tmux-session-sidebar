package config

import "testing"

func TestParseColorSchemeMode(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want ColorSchemeMode
	}{
		"empty defaults to system":     {raw: "", want: ColorSchemeModeSystem},
		"system":                       {raw: "system", want: ColorSchemeModeSystem},
		"light":                        {raw: "light", want: ColorSchemeModeLight},
		"dark":                         {raw: "dark", want: ColorSchemeModeDark},
		"unknown falls back to system": {raw: "sepia", want: ColorSchemeModeSystem},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ParseColorSchemeMode(tt.raw); got != tt.want {
				t.Fatalf("ParseColorSchemeMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveColorSchemeAppearance(t *testing.T) {
	tests := map[string]struct {
		mode   ColorSchemeMode
		system SystemColorSchemePreference
		want   ColorSchemeAppearance
	}{
		"system prefers light when unsupported": {mode: ColorSchemeModeSystem, system: SystemColorSchemeNoPreference, want: ColorSchemeAppearanceLight},
		"system follows dark preference":        {mode: ColorSchemeModeSystem, system: SystemColorSchemePreferDark, want: ColorSchemeAppearanceDark},
		"system follows light preference":       {mode: ColorSchemeModeSystem, system: SystemColorSchemePreferLight, want: ColorSchemeAppearanceLight},
		"force light ignores system":            {mode: ColorSchemeModeLight, system: SystemColorSchemePreferDark, want: ColorSchemeAppearanceLight},
		"force dark ignores system":             {mode: ColorSchemeModeDark, system: SystemColorSchemePreferLight, want: ColorSchemeAppearanceDark},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ResolveColorSchemeAppearance(tt.mode, tt.system); got != tt.want {
				t.Fatalf("ResolveColorSchemeAppearance(%q, %v) = %q, want %q", tt.mode, tt.system, got, tt.want)
			}
		})
	}
}

func TestParseSystemColorSchemePreference(t *testing.T) {
	tests := map[string]struct {
		raw  uint32
		want SystemColorSchemePreference
	}{
		"no preference":                   {raw: 0, want: SystemColorSchemeNoPreference},
		"prefer dark":                     {raw: 1, want: SystemColorSchemePreferDark},
		"prefer light":                    {raw: 2, want: SystemColorSchemePreferLight},
		"unknown clamps to no preference": {raw: 99, want: SystemColorSchemeNoPreference},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ParseSystemColorSchemePreference(tt.raw); got != tt.want {
				t.Fatalf("ParseSystemColorSchemePreference(%d) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
