package config

import "strings"

type ColorSchemeMode string

const (
	ColorSchemeModeSystem ColorSchemeMode = "system"
	ColorSchemeModeLight  ColorSchemeMode = "light"
	ColorSchemeModeDark   ColorSchemeMode = "dark"
)

type SystemColorSchemePreference uint32

const (
	SystemColorSchemeNoPreference SystemColorSchemePreference = 0
	SystemColorSchemePreferDark   SystemColorSchemePreference = 1
	SystemColorSchemePreferLight  SystemColorSchemePreference = 2
)

type ColorSchemeAppearance string

const (
	ColorSchemeAppearanceLight ColorSchemeAppearance = "light"
	ColorSchemeAppearanceDark  ColorSchemeAppearance = "dark"
)

func (p SystemColorSchemePreference) String() string {
	switch ParseSystemColorSchemePreference(uint32(p)) {
	case SystemColorSchemePreferDark:
		return "prefer-dark"
	case SystemColorSchemePreferLight:
		return "prefer-light"
	default:
		return "no-preference"
	}
}

func ParseColorSchemeMode(raw string) ColorSchemeMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(ColorSchemeModeSystem):
		return ColorSchemeModeSystem
	case string(ColorSchemeModeLight):
		return ColorSchemeModeLight
	case string(ColorSchemeModeDark):
		return ColorSchemeModeDark
	default:
		return ColorSchemeModeSystem
	}
}

func ParseSystemColorSchemePreference(raw uint32) SystemColorSchemePreference {
	switch SystemColorSchemePreference(raw) {
	case SystemColorSchemeNoPreference, SystemColorSchemePreferDark, SystemColorSchemePreferLight:
		return SystemColorSchemePreference(raw)
	default:
		return SystemColorSchemeNoPreference
	}
}

func ResolveColorSchemeAppearance(mode ColorSchemeMode, system SystemColorSchemePreference) ColorSchemeAppearance {
	switch ParseColorSchemeMode(string(mode)) {
	case ColorSchemeModeLight:
		return ColorSchemeAppearanceLight
	case ColorSchemeModeDark:
		return ColorSchemeAppearanceDark
	default:
		if ParseSystemColorSchemePreference(uint32(system)) == SystemColorSchemePreferDark {
			return ColorSchemeAppearanceDark
		}
		return ColorSchemeAppearanceLight
	}
}
