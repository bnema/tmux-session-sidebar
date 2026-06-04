package uity

import (
	"strings"
	"testing"
)

func TestBottomSheetOverlaysFullWidthAtBottom(t *testing.T) {
	base := strings.Join([]string{"one", "two", "three", "four", "five"}, "\n")
	sheet := BottomSheet{Title: "create session", Content: "> Git repo\n  Current dir", Footer: "esc cancel", Height: 6}

	view := stripANSI(sheet.RenderOverlay(base, 30, 8))
	lines := strings.Split(view, "\n")
	if len(lines) != 8 {
		t.Fatalf("height = %d, want 8: %q", len(lines), view)
	}
	if !strings.Contains(view, "create session") || !strings.Contains(view, "Git repo") {
		t.Fatalf("overlay missing sheet content: %q", view)
	}
	if !strings.Contains(view, "one") {
		t.Fatalf("overlay should preserve uncovered base content: %q", view)
	}
	for _, line := range lines[len(lines)-6:] {
		if metadataDisplayWidth(line) > 30 {
			t.Fatalf("bottom sheet line width = %d, want <= 30: %q", metadataDisplayWidth(line), line)
		}
	}
}

func TestBottomSheetClipsToBoundedHeight(t *testing.T) {
	base := strings.Join([]string{"1", "2", "3", "4", "5"}, "\n")
	sheet := BottomSheet{Title: "menu", Content: "a\nb\nc\nd\nz", Footer: "esc cancel", Height: 5}

	view := stripANSI(sheet.RenderOverlay(base, 20, 5))
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Fatalf("height = %d, want 5: %q", len(lines), view)
	}
	if strings.Count(view, "╭") != 1 || strings.Count(view, "╰") != 1 {
		t.Fatalf("expected preserved top and bottom sheet borders: %q", view)
	}
	if !strings.Contains(view, "esc cancel") {
		t.Fatalf("expected clipped sheet to preserve footer: %q", view)
	}
	if strings.Contains(view, "z") {
		t.Fatalf("expected tall content to be clipped before border rendering: %q", view)
	}
}
