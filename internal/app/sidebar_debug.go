package app

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

// logSidebarLayoutDebug emits best-effort debug logs for sidebar open/close
// layout actions when the sidebar adapter exposes ports.TmuxSidebarDebugPort.
// It records action and window keys plus optional client, pane, width, and
// snapshot or snapshot_error fields. The logger may be nil, missing debug-port
// support or an empty windowID short-circuit silently, and snapshot/logging
// errors are suppressed so debug instrumentation never affects normal flows.
func logSidebarLayoutDebug(ctx context.Context, sidebar ports.TmuxSidebarPort, action string, client string, paneID string, windowID string, width string) {
	debugger, ok := sidebar.(ports.TmuxSidebarDebugPort)
	windowID = strings.TrimSpace(windowID)
	if !ok || windowID == "" {
		return
	}
	cfg := loadSidebarConfig(ctx)
	_ = withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
		if logger == nil {
			return nil
		}
		fields := []ports.LogField{{Key: "action", Value: action}, {Key: "window", Value: windowID}}
		if trimmed := strings.TrimSpace(client); trimmed != "" {
			fields = append(fields, ports.LogField{Key: "client", Value: trimmed})
		}
		if trimmed := strings.TrimSpace(paneID); trimmed != "" {
			fields = append(fields, ports.LogField{Key: "pane", Value: trimmed})
		}
		if trimmed := strings.TrimSpace(width); trimmed != "" {
			fields = append(fields, ports.LogField{Key: "width", Value: trimmed})
		}
		snapshot, err := debugger.SidebarDebugSnapshot(ctx, windowID)
		if err != nil {
			logger.Error("sidebar-layout", append(fields, ports.LogField{Key: "snapshot_error", Value: err}))
			return nil
		}
		logger.Debug("sidebar-layout", append(fields, ports.LogField{Key: "snapshot", Value: snapshot}))
		return nil
	})
}
