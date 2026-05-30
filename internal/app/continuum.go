package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func shouldSkipSidebarSessionRestoreForContinuum(ctx context.Context, cfg ports.ConfigSnapshot) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.RestoreSessionsMode)) {
	case "off":
		return true
	case "on":
		return false
	}
	if !tmuxOptionBool(ctx, "@continuum-restore") {
		return false
	}
	if strings.TrimSpace(tmuxOptionValue(ctx, "@resurrect-restore-script-path")) == "" {
		return false
	}
	return tmuxServerStartedRecently(ctx, continuumRestoreWindowSeconds(ctx, cfg))
}

func continuumRestoreWindowSeconds(ctx context.Context, cfg ports.ConfigSnapshot) int64 {
	maxDelay := int64(tmuxOptionInt(ctx, "@continuum-restore-max-delay", 10))
	grace := max(int64(cfg.ContinuumGraceSeconds), 0)
	return maxDelay + grace
}

func tmuxServerStartedRecently(ctx context.Context, windowSeconds int64) bool {
	if windowSeconds <= 0 {
		return false
	}
	out, err := tmux(ctx, "display-message", "-p", "#{start_time}")
	if err != nil {
		return false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || start <= 0 {
		return false
	}
	now := time.Now().Unix()
	// Allow a small future skew for tmux start_time reads that race clock updates or
	// cross a short process/round-trip delay while the server is starting.
	return start >= now-windowSeconds && start <= now+5
}

func tmuxOptionBool(ctx context.Context, name string) bool {
	value := strings.ToLower(strings.TrimSpace(tmuxOptionValue(ctx, name)))
	switch value {
	case "1", "yes", "true", "on":
		return true
	default:
		return false
	}
}

func tmuxOptionInt(ctx context.Context, name string, fallback int) int {
	value := strings.TrimSpace(tmuxOptionValue(ctx, name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func tmuxOptionValue(ctx context.Context, name string) string {
	out, err := tmux(ctx, "show-options", "-gvq", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
