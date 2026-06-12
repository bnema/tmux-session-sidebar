package tmuxcli

import (
	"testing"
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
}
