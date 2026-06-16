package config

import "testing"

func TestParseAgentAttentionAnimation(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want AgentAttentionAnimation
	}{
		"empty defaults to pulse": {raw: "", want: AgentAttentionAnimationPulse},
		"pulse":                   {raw: "pulse", want: AgentAttentionAnimationPulse},
		"rainbow":                 {raw: "rainbow", want: AgentAttentionAnimationRainbow},
		"blink":                   {raw: "blink", want: AgentAttentionAnimationBlink},
		"off":                     {raw: "off", want: AgentAttentionAnimationOff},
		"unknown disables":        {raw: "sparkle", want: AgentAttentionAnimationOff},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ParseAgentAttentionAnimation(tt.raw); got != tt.want {
				t.Fatalf("ParseAgentAttentionAnimation(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
