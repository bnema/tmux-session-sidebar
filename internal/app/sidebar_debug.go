package app

import (
	"context"
	"strings"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func logSidebarLayoutDebug(ctx context.Context, sidebar ports.TmuxSidebarPort, action string, client string, paneID string, windowID string, width string) {
	debugger, ok := sidebar.(ports.TmuxSidebarDebugPort)
	if !ok || strings.TrimSpace(windowID) == "" {
		return
	}
	cfg := loadSidebarConfig(ctx)
	_ = withActivityDebugLogger(cfg, func(logger ports.LoggerPort) error {
		if logger == nil {
			return nil
		}
		fields := []ports.LogField{{Key: "action", Value: action}, {Key: "window", Value: strings.TrimSpace(windowID)}}
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
