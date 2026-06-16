package uity

import (
	"fmt"
	"math"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
)

type rgbColor struct {
	red   int
	green int
	blue  int
}

type sidebarColorScheme struct {
	accent                      string
	dim                         string
	treeGuide                   string
	active                      string
	selectedForeground          string
	warning                     string
	destructive                 string
	versionBadgeBackground      string
	versionBadgeForeground      string
	updateIndicatorBackground   string
	updateIndicatorForeground   string
	bottomSheetTitle            string
	bottomSheetFooter           string
	pinPickerBackground         string
	pinPickerBorder             string
	pinPickerTitle              string
	pinPickerHint               string
	selectedRowBackgroundRGB    rgbColor
	attentionBackgroundRGB      rgbColor
	inactiveSessionRGB          rgbColor
	freshInactiveSessionRGB     rgbColor
	inactiveMetadataRGB         rgbColor
	freshInactiveMetadataRGB    rgbColor
	selectedInactiveMetadataRGB rgbColor
	heatCoolRGB                 rgbColor
	heatHotRGB                  rgbColor
	metadataBase                string
	metadataSelectedMuted       string
	metadataSeparator           string
	metadataSelectedSeparator   string
	metadataActive              metadataColors
	metadataSelectedActive      metadataColors
}

var darkMode = sidebarColorScheme{
	accent:                      "#86efac",
	dim:                         "#6b7280",
	treeGuide:                   "#333333",
	active:                      "#ffffff",
	selectedForeground:          "#ecfdf5",
	warning:                     "#fbbf24",
	destructive:                 "#f87171",
	versionBadgeBackground:      "#334155",
	versionBadgeForeground:      "#e0f2fe",
	updateIndicatorBackground:   "#334155",
	updateIndicatorForeground:   "#22c55e",
	bottomSheetTitle:            "#e0f2fe",
	bottomSheetFooter:           "#64748b",
	pinPickerBackground:         "#0f172a",
	pinPickerBorder:             "#065f46",
	pinPickerTitle:              "#e0f2fe",
	pinPickerHint:               "#6b7280",
	selectedRowBackgroundRGB:    rgbColor{red: 6, green: 95, blue: 70},
	attentionBackgroundRGB:      rgbColor{},
	inactiveSessionRGB:          rgbColor{red: 75, green: 85, blue: 99},
	freshInactiveSessionRGB:     rgbColor{red: 204, green: 204, blue: 204},
	inactiveMetadataRGB:         rgbColor{red: 55, green: 65, blue: 81},
	freshInactiveMetadataRGB:    rgbColor{red: 184, green: 184, blue: 186},
	selectedInactiveMetadataRGB: rgbColor{red: 148, green: 163, blue: 184},
	heatCoolRGB:                 rgbColor{red: 22, green: 101, blue: 52},
	heatHotRGB:                  rgbColor{red: 240, green: 253, blue: 244},
	metadataBase:                "#cccccc",
	metadataSelectedMuted:       "#94a3b8",
	metadataSeparator:           "#64748b",
	metadataSelectedSeparator:   "#94a3b8",
	metadataActive:              metadataColors{compare: "#38bdf8", ahead: "#4ade80", behind: "#f87171", staged: "#60a5fa", unstaged: "#d6c86f"},
	metadataSelectedActive:      metadataColors{compare: "#7dd3fc", ahead: "#86efac", behind: "#f87171", staged: "#93c5fd", unstaged: "#e4d987"},
}

var lightMode = sidebarColorScheme{
	accent:                      "#166534",
	dim:                         "#64748b",
	treeGuide:                   "#94a3b8",
	active:                      "#0f172a",
	selectedForeground:          "#064e3b",
	warning:                     "#b45309",
	destructive:                 "#dc2626",
	versionBadgeBackground:      "#cbd5e1",
	versionBadgeForeground:      "#0f172a",
	updateIndicatorBackground:   "#cbd5e1",
	updateIndicatorForeground:   "#15803d",
	bottomSheetTitle:            "#0f172a",
	bottomSheetFooter:           "#64748b",
	pinPickerBackground:         "#f8fafc",
	pinPickerBorder:             "#6ee7b7",
	pinPickerTitle:              "#0f172a",
	pinPickerHint:               "#64748b",
	selectedRowBackgroundRGB:    rgbColor{red: 167, green: 243, blue: 208},
	attentionBackgroundRGB:      rgbColor{red: 248, green: 250, blue: 252},
	inactiveSessionRGB:          rgbColor{red: 148, green: 163, blue: 184},
	freshInactiveSessionRGB:     rgbColor{red: 51, green: 65, blue: 85},
	inactiveMetadataRGB:         rgbColor{red: 100, green: 116, blue: 139},
	freshInactiveMetadataRGB:    rgbColor{red: 30, green: 41, blue: 59},
	selectedInactiveMetadataRGB: rgbColor{red: 6, green: 78, blue: 59},
	heatCoolRGB:                 rgbColor{red: 34, green: 139, blue: 94},
	heatHotRGB:                  rgbColor{red: 22, green: 101, blue: 52},
	metadataBase:                "#475569",
	metadataSelectedMuted:       "#065f46",
	metadataSeparator:           "#94a3b8",
	metadataSelectedSeparator:   "#065f46",
	metadataActive:              metadataColors{compare: "#0369a1", ahead: "#15803d", behind: "#dc2626", staged: "#1d4ed8", unstaged: "#a16207"},
	metadataSelectedActive:      metadataColors{compare: "#0c4a6e", ahead: "#166534", behind: "#991b1b", staged: "#1e40af", unstaged: "#854d0e"},
}

func colorScheme(appearance config.ColorSchemeAppearance) sidebarColorScheme {
	if appearance == config.ColorSchemeAppearanceLight {
		return lightMode
	}
	return darkMode
}

func heatColor(intensity float64) string {
	return heatColorForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func heatColorForAppearance(appearance config.ColorSchemeAppearance, intensity float64) string {
	return heatRGBForAppearance(appearance, intensity).Hex()
}

func heatRGB(intensity float64) rgbColor {
	return heatRGBForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func heatRGBForAppearance(appearance config.ColorSchemeAppearance, intensity float64) rgbColor {
	scheme := colorScheme(appearance)
	return blendRGB(scheme.heatCoolRGB, scheme.heatHotRGB, clampIntensity(intensity))
}

func inactiveSessionColor(intensity float64) string {
	return inactiveSessionColorForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func inactiveSessionColorForAppearance(appearance config.ColorSchemeAppearance, intensity float64) string {
	return inactiveSessionRGBForAppearance(appearance, intensity).Hex()
}

func inactiveSessionRGBForIntensity(intensity float64) rgbColor {
	return inactiveSessionRGBForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func inactiveSessionRGBForAppearance(appearance config.ColorSchemeAppearance, intensity float64) rgbColor {
	scheme := colorScheme(appearance)
	return blendRGB(scheme.inactiveSessionRGB, scheme.freshInactiveSessionRGB, clampIntensity(intensity))
}

func inactiveMetadataColor(intensity float64) string {
	return inactiveMetadataColorForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func inactiveMetadataColorForAppearance(appearance config.ColorSchemeAppearance, intensity float64) string {
	return inactiveMetadataRGBForAppearance(appearance, intensity).Hex()
}

func inactiveMetadataRGBForIntensity(intensity float64) rgbColor {
	return inactiveMetadataRGBForAppearance(config.ColorSchemeAppearanceDark, intensity)
}

func inactiveMetadataRGBForAppearance(appearance config.ColorSchemeAppearance, intensity float64) rgbColor {
	scheme := colorScheme(appearance)
	return blendRGB(scheme.inactiveMetadataRGB, scheme.freshInactiveMetadataRGB, clampIntensity(intensity))
}

func hslHex(hue, saturation, lightness float64) string {
	return hslRGB(hue, saturation, lightness).Hex()
}

func hslRGB(hue, saturation, lightness float64) rgbColor {
	hue = math.Mod(hue, 360)
	if hue < 0 {
		hue += 360
	}
	saturation = clampIntensity(saturation)
	lightness = clampIntensity(lightness)
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	h := hue / 60
	x := chroma * (1 - math.Abs(math.Mod(h, 2)-1))
	red, green, blue := 0.0, 0.0, 0.0
	switch {
	case h < 1:
		red, green = chroma, x
	case h < 2:
		red, green = x, chroma
	case h < 3:
		green, blue = chroma, x
	case h < 4:
		green, blue = x, chroma
	case h < 5:
		red, blue = x, chroma
	default:
		red, blue = chroma, x
	}
	match := lightness - chroma/2
	return rgbColor{red: colorByte(red + match), green: colorByte(green + match), blue: colorByte(blue + match)}
}

func colorByte(value float64) int {
	return int(math.Round(clampIntensity(value) * 255))
}

func blendRGB(cool rgbColor, hot rgbColor, intensity float64) rgbColor {
	return rgbColor{
		red:   blendChannel(cool.red, hot.red, intensity),
		green: blendChannel(cool.green, hot.green, intensity),
		blue:  blendChannel(cool.blue, hot.blue, intensity),
	}
}

func blendChannel(cool int, hot int, intensity float64) int {
	return int(math.Round(float64(cool) + float64(hot-cool)*intensity))
}

func clampIntensity(intensity float64) float64 {
	if intensity < 0 {
		return 0
	}
	if intensity > 1 {
		return 1
	}
	return intensity
}

func (c rgbColor) ANSI() string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.red, c.green, c.blue)
}

func (c rgbColor) DimANSI() string {
	return fmt.Sprintf("\033[2;38;2;%d;%d;%dm", c.red, c.green, c.blue)
}

func (c rgbColor) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.red, c.green, c.blue)
}
