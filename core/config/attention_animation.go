package config

import "strings"

type AgentAttentionAnimation string

const (
	AgentAttentionAnimationOff     AgentAttentionAnimation = "off"
	AgentAttentionAnimationPulse   AgentAttentionAnimation = "pulse"
	AgentAttentionAnimationRainbow AgentAttentionAnimation = "rainbow"
	AgentAttentionAnimationBlink   AgentAttentionAnimation = "blink"
)

func ParseAgentAttentionAnimation(raw string) AgentAttentionAnimation {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(AgentAttentionAnimationPulse):
		return AgentAttentionAnimationPulse
	case string(AgentAttentionAnimationRainbow):
		return AgentAttentionAnimationRainbow
	case string(AgentAttentionAnimationBlink):
		return AgentAttentionAnimationBlink
	case string(AgentAttentionAnimationOff):
		return AgentAttentionAnimationOff
	default:
		return AgentAttentionAnimationOff
	}
}
