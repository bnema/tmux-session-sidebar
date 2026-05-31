package uity

import (
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

func TestPulseAnimationBlendsFromBackgroundToWhite(t *testing.T) {
	background := rgbColor{red: 6, green: 95, blue: 70}
	if _, ok := attentionAnimationColor(config.AgentAttentionAnimationPulse, 0, background); ok {
		t.Fatal("transparent pulse frame returned a foreground color")
	}
	colors := attentionAnimationColors(config.AgentAttentionAnimationPulse, background)
	if colors[0] != background.Hex() {
		t.Fatalf("first generated pulse color = %q, want background %q", colors[0], background.Hex())
	}

	peakFrame := attentionAnimationFrameCount(config.AgentAttentionAnimationPulse) / 2
	peak, ok := attentionAnimationColor(config.AgentAttentionAnimationPulse, peakFrame, background)
	if !ok {
		t.Fatal("pulse animation did not return a peak color")
	}
	if peak != "#ffffff" {
		t.Fatalf("peak pulse color = %q, want white", peak)
	}

	last, ok := attentionAnimationColor(config.AgentAttentionAnimationPulse, attentionAnimationFrameCount(config.AgentAttentionAnimationPulse)-1, background)
	if !ok {
		t.Fatal("pulse animation did not return a final color")
	}
	second, _ := attentionAnimationColor(config.AgentAttentionAnimationPulse, 1, background)
	if last != second {
		t.Fatalf("last pulse color = %q, want symmetric ramp down to %q", last, second)
	}
	interval := attentionAnimationInterval(config.AgentAttentionAnimationPulse)
	if interval > 200*time.Millisecond {
		t.Fatalf("pulse interval = %v, want <= 200ms for smooth breathing", interval)
	}
	cycle := time.Duration(attentionAnimationFrameCount(config.AgentAttentionAnimationPulse)) * interval
	if cycle < 3500*time.Millisecond {
		t.Fatalf("pulse cycle = %v, want at least 3.5s for calmer breathing", cycle)
	}
}

func TestRainbowAnimationUsesGeneratedHueWheel(t *testing.T) {
	colors := attentionAnimationColors(config.AgentAttentionAnimationRainbow, rgbColor{})
	if len(colors) != attentionRainbowFrameCount {
		t.Fatalf("rainbow frame count = %d, want %d", len(colors), attentionRainbowFrameCount)
	}
	if colors[0] != hslHex(0, attentionRainbowSaturation, attentionRainbowLightness) {
		t.Fatalf("first rainbow color = %q, want generated red hue", colors[0])
	}
	if colors[1] == colors[0] {
		t.Fatalf("rainbow did not advance hue: %q then %q", colors[0], colors[1])
	}
	interval := attentionAnimationInterval(config.AgentAttentionAnimationRainbow)
	if interval > 200*time.Millisecond {
		t.Fatalf("rainbow interval = %v, want <= 200ms for a playful color cycle", interval)
	}
	cycle := time.Duration(attentionAnimationFrameCount(config.AgentAttentionAnimationRainbow)) * interval
	if cycle > 2500*time.Millisecond {
		t.Fatalf("rainbow cycle = %v, want <= 2.5s for an energetic full loop", cycle)
	}
}
