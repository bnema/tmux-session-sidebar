package uity

import (
	"fmt"
	"math"
)

type rgbColor struct {
	red   int
	green int
	blue  int
}

var (
	// selectedRowBackgroundRGB is the sidebar selection background (#065f46).
	selectedRowBackgroundRGB = rgbColor{red: 6, green: 95, blue: 70}
	// inactiveSessionRGB is the out-of-heat color: the default dark gray (#4b5563)
	// used only for sessions outside the recent activity window.
	inactiveSessionRGB = rgbColor{red: 75, green: 85, blue: 99}
	// freshInactiveSessionRGB is the light endpoint for stale/non-heat sessions
	// so the freshest inactive row can lift toward a soft gray (#cccccc).
	freshInactiveSessionRGB = rgbColor{red: 204, green: 204, blue: 204}
	// inactiveMetadataRGB is darker than inactive sessions so metadata remains
	// visually secondary for dead/inactive rows (#374151).
	inactiveMetadataRGB = rgbColor{red: 55, green: 65, blue: 81}
	// freshInactiveMetadataRGB tracks the inactive session lift while staying a
	// little darker than the session label (#b8b8ba).
	freshInactiveMetadataRGB = rgbColor{red: 184, green: 184, blue: 186}
	// selectedInactiveMetadataRGB keeps inactive metadata readable on the
	// selected emerald row background without promoting it to active colors (#94a3b8).
	selectedInactiveMetadataRGB = rgbColor{red: 148, green: 163, blue: 184}
	// heatCoolRGB is the low-intensity heat endpoint: a dark green (#166534)
	// used when recent activity is near the edge of the configured heat window.
	heatCoolRGB = rgbColor{red: 22, green: 101, blue: 52}
	// heatHotRGB is the high-intensity endpoint: a nearly-white light green
	// (#f0fdf4) used for the most recent activity.
	heatHotRGB = rgbColor{red: 240, green: 253, blue: 244}
)

func heatColor(intensity float64) string {
	return heatRGB(intensity).Hex()
}

func heatRGB(intensity float64) rgbColor {
	return blendRGB(heatCoolRGB, heatHotRGB, clampIntensity(intensity))
}

func inactiveSessionColor(intensity float64) string {
	return inactiveSessionRGBForIntensity(intensity).Hex()
}

func inactiveSessionRGBForIntensity(intensity float64) rgbColor {
	return blendRGB(inactiveSessionRGB, freshInactiveSessionRGB, clampIntensity(intensity))
}

func inactiveMetadataColor(intensity float64) string {
	return inactiveMetadataRGBForIntensity(intensity).Hex()
}

func inactiveMetadataRGBForIntensity(intensity float64) rgbColor {
	return blendRGB(inactiveMetadataRGB, freshInactiveMetadataRGB, clampIntensity(intensity))
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
	return cool + int(float64(hot-cool)*intensity+0.5)
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
