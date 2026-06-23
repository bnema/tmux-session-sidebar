package tmuxcli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func TestSyncAttachedSidebarWidthUsesSavedBaselineProportions(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)
	baseline := `{"representativePaneIDs":["%27","%185"],"workWidths":[74,75]}`

	rec.handle([]string{"show-options", "-w", "-v", "-t", "@27", optionSidebarOpenWorkBaseline}, func([]string) (string, string) {
		return baseline, ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@27", optionSidebarResizeSyncActive, "1"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%183", "-x", "30"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@27", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@27", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%183\t0\t0\t30\t48\t1\n%27\t31\t0\t91\t48\t0\n%185\t123\t0\t58\t24\t0\n%186\t123\t25\t58\t23\t0", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%27", "-x", "74"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@27", optionSidebarResizeSyncActive}, func([]string) (string, string) {
		return "", ""
	})

	if err := (Client{Process: rec}).SyncAttachedSidebarWidth(ctx, "@27", "%183", "30"); err != nil {
		t.Fatalf("SyncAttachedSidebarWidth error: %v", err)
	}
	assertRecUsedAllHandlers(t, rec)
}

func TestSyncAttachedSidebarWidthLogsBaselineAndComputedWidths(t *testing.T) {
	ctx := t.Context()
	logger := &recordingResizeLogger{}
	rec := newRecPort(t)
	baseline := `{"representativePaneIDs":["%27","%185"],"workWidths":[74,75]}`

	rec.handle([]string{"show-options", "-w", "-v", "-t", "@27", optionSidebarOpenWorkBaseline}, func([]string) (string, string) {
		return baseline, ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@27", optionSidebarResizeSyncActive, "1"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%183", "-x", "30"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@27", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@27", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%183\t0\t0\t30\t48\t1\n%27\t31\t0\t91\t48\t0\n%185\t123\t0\t58\t24\t0\n%186\t123\t25\t58\t23\t0", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%27", "-x", "74"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@27", optionSidebarResizeSyncActive}, func([]string) (string, string) {
		return "", ""
	})

	if err := (Client{Process: rec, Logger: logger}).SyncAttachedSidebarWidth(ctx, "@27", "%183", "30"); err != nil {
		t.Fatalf("SyncAttachedSidebarWidth error: %v", err)
	}
	log := logger.joined()
	for _, want := range []string{
		"debug: resize-sync-start window=@27 pane=%183 width=30",
		"debug: resize-baseline-loaded window=@27 representatives=[%27 %185] widths=[74 75]",
		"debug: resize-work-groups window=@27 sidebar=%183 require_sidebar=true expected_sidebar_width=0 window_width=181 window_height=48 panes=[%183@0,0 30x48 sidebar=true %27@31,0 91x48 sidebar=false %185@123,0 58x24 sidebar=false %186@123,25 58x23 sidebar=false] groups=[%27 left=31 width=91 top=0 bottom=48 uniform=true %185 left=123 width=58 top=0 bottom=48 uniform=true]",
		"debug: resize-work-weights window=@27 sidebar=%183 total_width=149 weights=[%27=74 %185=75] target_widths=[74 75]",
		"debug: resize-work-pane pane=%27 from=91 to=74",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("resize log missing %q:\n%s", want, log)
		}
	}
	assertRecUsedAllHandlers(t, rec)
}

func TestSyncAttachedSidebarWidthSkipsRestoreWhenSavedBaselineNoLongerMatchesTopology(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)
	baseline := `{"representativePaneIDs":["%27","%185"],"workWidths":[74,75]}`

	rec.handle([]string{"show-options", "-w", "-v", "-t", "@27", optionSidebarOpenWorkBaseline}, func([]string) (string, string) {
		return baseline, ""
	})
	rec.handle([]string{"set-option", "-wq", "-t", "@27", optionSidebarResizeSyncActive, "1"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"resize-pane", "-t", "%183", "-x", "30"}, func([]string) (string, string) {
		return "", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@27", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@27", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%183\t0\t0\t30\t48\t1\n%300\t31\t0\t91\t48\t0\n%185\t123\t0\t58\t24\t0\n%186\t123\t25\t58\t23\t0", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@27", optionSidebarResizeSyncActive}, func([]string) (string, string) {
		return "", ""
	})

	if err := (Client{Process: rec}).SyncAttachedSidebarWidth(ctx, "@27", "%183", "30"); err != nil {
		t.Fatalf("SyncAttachedSidebarWidth error: %v", err)
	}
	assertRecUsedAllHandlers(t, rec)
}

func TestCaptureAttachedSidebarWidthBaselineClearsSavedBaselineWhenCaptureIsInvalid(t *testing.T) {
	ctx := t.Context()
	rec := newRecPort(t)

	rec.handle([]string{"show-options", "-w", "-v", "-t", "@27", optionSidebarResizeSyncActive}, func([]string) (string, string) {
		return "0", ""
	})
	rec.handle([]string{"display-message", "-p", "-t", "@27", "#{window_width}\t#{window_height}"}, func([]string) (string, string) {
		return "181\t48", ""
	})
	rec.handle([]string{"list-panes", "-t", "@27", "-F", formatSidebarRebalancePane}, func([]string) (string, string) {
		return "%183\t0\t0\t30\t48\t1\n%27\t31\t0\t150\t48\t0", ""
	})
	rec.handle([]string{"set-option", "-wu", "-t", "@27", optionSidebarOpenWorkBaseline}, func([]string) (string, string) {
		return "", ""
	})

	if err := (Client{Process: rec}).CaptureAttachedSidebarWidthBaseline(ctx, "@27", "%183", "30"); err != nil {
		t.Fatalf("CaptureAttachedSidebarWidthBaseline error: %v", err)
	}
	assertRecUsedAllHandlers(t, rec)
}

type recordingResizeLogger struct {
	lines []string
}

func (l *recordingResizeLogger) Debug(msg string, fields []ports.LogField) {
	l.lines = append(l.lines, recordingResizeLogLine("debug", msg, fields))
}

func (l *recordingResizeLogger) Info(msg string, fields []ports.LogField) {
	l.lines = append(l.lines, recordingResizeLogLine("info", msg, fields))
}

func (l *recordingResizeLogger) Error(msg string, fields []ports.LogField) {
	l.lines = append(l.lines, recordingResizeLogLine("error", msg, fields))
}

func (l *recordingResizeLogger) joined() string {
	return strings.Join(l.lines, "\n")
}

func recordingResizeLogLine(level string, msg string, fields []ports.LogField) string {
	var line strings.Builder
	_, _ = fmt.Fprintf(&line, "%s: %s", level, msg)
	for _, field := range fields {
		_, _ = fmt.Fprintf(&line, " %s=%v", field.Key, field.Value)
	}
	return line.String()
}

func assertRecUsedAllHandlers(t *testing.T, rec *recProcessPort) {
	t.Helper()
	if len(rec.calls) != len(rec.handlers) {
		t.Fatalf("recorded tmux calls = %d, handlers = %d\ncalls: %#v", len(rec.calls), len(rec.handlers), rec.calls)
	}
}
