package uity

import (
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

type attentionAnimationTickMsg struct {
	Generation int
}

const untrackedAttentionAnimationTick = -1

const (
	pulseRampFrameCount        = 30
	attentionRainbowFrameCount = 12
	attentionBlinkFrameCount   = 2
	attentionRainbowSaturation = 0.85
	attentionRainbowLightness  = 0.62
)

var (
	defaultAttentionBackgroundRGB = rgbColor{}
	whiteRGB                      = rgbColor{red: 255, green: 255, blue: 255}
)

func normalizeAttentionAnimation(style config.AgentAttentionAnimation) config.AgentAttentionAnimation {
	return config.ParseAgentAttentionAnimation(string(style))
}

func attentionAnimationEnabled(style config.AgentAttentionAnimation) bool {
	return normalizeAttentionAnimation(style) != config.AgentAttentionAnimationOff
}

func hasAttention(items []SessionItem) bool {
	for _, item := range items {
		if item.Attention {
			return true
		}
	}
	return false
}

func shouldRunAttentionAnimation(style config.AgentAttentionAnimation, items []SessionItem) bool {
	return attentionAnimationEnabled(style) && hasAttention(items)
}

func attentionAnimationTickCmd(style config.AgentAttentionAnimation, items []SessionItem, generation int) tea.Cmd {
	style = normalizeAttentionAnimation(style)
	if !shouldRunAttentionAnimation(style, items) {
		return nil
	}
	return tea.Tick(attentionAnimationInterval(style), func(time.Time) tea.Msg {
		return attentionAnimationTickMsg{Generation: generation}
	})
}

func batchCommands(cmds ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return tea.Batch(filtered...)
}

func nextAttentionAnimationFrame(style config.AgentAttentionAnimation, frame int) int {
	count := attentionAnimationFrameCount(style)
	if count <= 0 {
		return 0
	}
	return (frame + 1) % count
}

func attentionAnimationFrameCount(style config.AgentAttentionAnimation) int {
	switch normalizeAttentionAnimation(style) {
	case config.AgentAttentionAnimationPulse:
		return pulseRampFrameCount
	case config.AgentAttentionAnimationRainbow:
		return attentionRainbowFrameCount
	case config.AgentAttentionAnimationBlink:
		return attentionBlinkFrameCount
	default:
		return 1
	}
}

func attentionAnimationInterval(style config.AgentAttentionAnimation) time.Duration {
	switch normalizeAttentionAnimation(style) {
	case config.AgentAttentionAnimationRainbow:
		return 150 * time.Millisecond
	case config.AgentAttentionAnimationBlink:
		return 900 * time.Millisecond
	case config.AgentAttentionAnimationPulse:
		return 120 * time.Millisecond
	default:
		return time.Second
	}
}

func animatedAttentionMarkerStyle(base lipgloss.Style, style config.AgentAttentionAnimation, frame int, background rgbColor) lipgloss.Style {
	color, ok := attentionAnimationColor(style, frame, background)
	if !ok {
		return base
	}
	return base.Foreground(lipgloss.Color(color))
}

func animatedAttentionMarkerSymbol(symbol string, style config.AgentAttentionAnimation, frame int) string {
	if attentionAnimationTransparentFrame(style, frame) {
		return " "
	}
	return symbol
}

func attentionAnimationColor(style config.AgentAttentionAnimation, frame int, background rgbColor) (string, bool) {
	if attentionAnimationTransparentFrame(style, frame) {
		return "", false
	}
	style = normalizeAttentionAnimation(style)
	frame = positiveModulo(frame, attentionAnimationFrameCount(style))
	switch style {
	case config.AgentAttentionAnimationPulse:
		return pulseFrameColor(background, whiteRGB, frame, pulseRampFrameCount).Hex(), true
	case config.AgentAttentionAnimationRainbow:
		return hslHex(float64(frame)*360/float64(attentionRainbowFrameCount), attentionRainbowSaturation, attentionRainbowLightness), true
	case config.AgentAttentionAnimationBlink:
		return pulseFrameColor(background, whiteRGB, frame, attentionBlinkFrameCount).Hex(), true
	default:
		return "", false
	}
}

func attentionAnimationTransparentFrame(style config.AgentAttentionAnimation, frame int) bool {
	switch normalizeAttentionAnimation(style) {
	case config.AgentAttentionAnimationPulse, config.AgentAttentionAnimationBlink:
		return positiveModulo(frame, attentionAnimationFrameCount(style)) == 0
	default:
		return false
	}
}

func attentionAnimationColors(style config.AgentAttentionAnimation, background rgbColor) []string {
	switch normalizeAttentionAnimation(style) {
	case config.AgentAttentionAnimationPulse:
		return pulseColorRamp(background, whiteRGB, pulseRampFrameCount)
	case config.AgentAttentionAnimationRainbow:
		return hueWheelRamp(attentionRainbowFrameCount, attentionRainbowSaturation, attentionRainbowLightness)
	case config.AgentAttentionAnimationBlink:
		return pulseColorRamp(background, whiteRGB, attentionBlinkFrameCount)
	default:
		return nil
	}
}

func pulseColorRamp(base rgbColor, peak rgbColor, frameCount int) []string {
	if frameCount <= 0 {
		return nil
	}
	colors := make([]string, 0, frameCount)
	for frame := range frameCount {
		colors = append(colors, pulseFrameColor(base, peak, frame, frameCount).Hex())
	}
	return colors
}

func pulseFrameColor(base rgbColor, peak rgbColor, frame int, frameCount int) rgbColor {
	if frameCount <= 1 {
		return peak
	}
	peakIndex := frameCount / 2
	distanceFromPeak := absInt(frame - peakIndex)
	intensity := 1 - float64(distanceFromPeak)/float64(peakIndex)
	return blendRGB(base, peak, intensity)
}

func hueWheelRamp(frameCount int, saturation float64, lightness float64) []string {
	if frameCount <= 0 {
		return nil
	}
	colors := make([]string, 0, frameCount)
	for frame := range frameCount {
		colors = append(colors, hslHex(float64(frame)*360/float64(frameCount), saturation, lightness))
	}
	return colors
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func positiveModulo(value int, modulo int) int {
	if modulo <= 0 {
		return 0
	}
	result := value % modulo
	if result < 0 {
		return result + modulo
	}
	return result
}
