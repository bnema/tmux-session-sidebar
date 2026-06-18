package uity

import (
	"reflect"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestPinColorPaletteUsesDistinctCuratedColors(t *testing.T) {
	want := []string{
		defaultPinColor,
		"#f97316",
		"#ef4444",
		"#ec4899",
		"#a855f7",
		"#6366f1",
		"#3b82f6",
		"#06b6d4",
		"#14b8a6",
		"#22c55e",
		"#84cc16",
	}
	if !reflect.DeepEqual(pinColorPalette, want) {
		t.Fatalf("pinColorPalette = %#v, want %#v", pinColorPalette, want)
	}
}

func TestPinColorPickerResetOptionClearsSelectedColor(t *testing.T) {
	picker := PinColorPicker{Cursor: len(pinColorPalette)}
	if got := picker.SelectedColor(); got != "" {
		t.Fatalf("SelectedColor() at reset option = %q, want empty string", got)
	}
	view := stripANSI(picker.Render())
	if !strings.Contains(view, pinColorResetLabel) {
		t.Fatalf("picker view missing reset option tile: %q", view)
	}
	for _, want := range []string{"↵/sp apply", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker view missing hint %q: %q", want, view)
		}
	}
	if strings.Contains(view, "auto reset") {
		t.Fatalf("picker footer should not repeat reset hint: %q", view)
	}
	if width := lipgloss.Width(view); width > 20 {
		t.Fatalf("picker width = %d, want <= 20 for default narrow sidebar", width)
	}
}

func TestPinColorPickerMoveWrapsToResetOption(t *testing.T) {
	picker := PinColorPicker{}
	picker.Cursor = picker.Move(-1)
	if picker.Cursor != picker.resetIndex() {
		t.Fatalf("wrapped cursor = %d, want reset index %d", picker.Cursor, picker.resetIndex())
	}
	if got := picker.SelectedColor(); got != "" {
		t.Fatalf("SelectedColor() after wrap = %q, want empty reset color", got)
	}
}

func TestPinColorPickerRenderUsesConfiguredLightAppearance(t *testing.T) {
	view := PinColorPicker{Appearance: "light"}.Render()
	if !strings.Contains(view, "48;2;248;250;252") {
		t.Fatalf("light picker render missing light background, view=%q", view)
	}
	if strings.Contains(view, "48;2;15;23;42") {
		t.Fatalf("light picker render used dark background, view=%q", view)
	}
}
