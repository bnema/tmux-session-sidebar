package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/core/attention"
	coreruntime "github.com/bnema/tmux-session-sidebar/core/runtime"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func recordAgentHookEvent(ctx context.Context, flags map[string]string) error {
	cfg := loadSidebarConfig(ctx)
	if !cfg.AgentAttentionEnabled {
		return nil
	}
	event, err := agentEventFromFlags(ctx, flags)
	if err != nil {
		return err
	}
	refresh := false
	if err := withLockedSidebarStore(ctx, func(store storefs.Store) error {
		return withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
			if logger != nil {
				logger.Debug("agent-event", []ports.LogField{
					{Key: "agent", Value: event.Agent},
					{Key: "action", Value: event.Action},
					{Key: "session", Value: event.SessionName},
					{Key: "pane", Value: event.PaneID},
				})
			}
			changed, err := runtimeServiceWithStore(store).WithLogger(logger).RecordAgentAttentionEvent(ctx, "tmux", event)
			refresh = changed
			return err
		})
	}); err != nil {
		return err
	}
	if refresh {
		refreshAllSidebarPanesBestEffort(ctx)
	}
	return nil
}

func agentEventFromFlags(ctx context.Context, flags map[string]string) (coreruntime.AgentAttentionEvent, error) {
	action, err := agentAttentionAction(flags["event"])
	if err != nil {
		return coreruntime.AgentAttentionEvent{}, err
	}
	paneID := strings.TrimSpace(flags["pane"])
	if paneID == "" {
		paneID = strings.TrimSpace(flags["pane-id"])
	}
	if paneID == "" {
		return coreruntime.AgentAttentionEvent{}, fmt.Errorf("agent hook event requires --pane")
	}

	sessionName := strings.TrimSpace(flags["session"])
	sessionID := strings.TrimSpace(flags["session-id"])
	if sessionName == "" || sessionID == "" {
		output, err := tmux(ctx, "display-message", "-p", "-t", paneID, "#{session_name}\t#{session_id}")
		if err != nil {
			return coreruntime.AgentAttentionEvent{}, tmuxCommandError("resolve agent hook pane", output, err)
		}
		parts := strings.Split(strings.TrimSpace(output), "\t")
		if sessionName == "" && len(parts) > 0 {
			sessionName = strings.TrimSpace(parts[0])
		}
		if sessionID == "" && len(parts) > 1 {
			sessionID = strings.TrimSpace(parts[1])
		}
	}
	if sessionName == "" {
		return coreruntime.AgentAttentionEvent{}, fmt.Errorf("agent hook event could not resolve session for pane %s", paneID)
	}

	return coreruntime.AgentAttentionEvent{
		Action:      action,
		Agent:       strings.TrimSpace(flags["agent"]),
		SessionName: sessionName,
		SessionID:   sessionID,
		PaneID:      paneID,
		OccurredAt:  time.Now().UTC(),
	}, nil
}

func agentAttentionAction(raw string) (attention.Action, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "noop", "shell-done":
		return attention.ActionNoop, nil
	case "session-start", "prompt-submit", "shell-exec", "start", "running":
		return attention.ActionRunning, nil
	case "stop", "notification", "notify", "agent-response", "attention", "end", "complete", "completed":
		return attention.ActionAttention, nil
	case "session-end":
		return attention.ActionSessionEnd, nil
	default:
		return attention.ActionNoop, fmt.Errorf("agent hook event requires a supported --event value")
	}
}
