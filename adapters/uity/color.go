package uity

import "fmt"

type rgbColor struct {
	red   int
	green int
	blue  int
}

var (
	// inactiveSessionRGB is the out-of-heat color: the default dark gray (#4b5563)
	// used only for sessions outside the recent activity window.
	inactiveSessionRGB = rgbColor{red: 75, green: 85, blue: 99}
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
