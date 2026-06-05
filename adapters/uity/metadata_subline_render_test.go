package uity

import (
	"strings"
	"testing"
)

func TestRenderMetadataSublineColorsActiveGitParts(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Selected: true, Active: true})

	for _, want := range []string{"38;2;125;211;252", "38;2;134;239;172", "38;2;248;113;113", "38;2;147;197;253"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderMetadataSubline() should include color %s, got %q", want, got)
		}
	}
	if strings.Contains(got, "38;2;253;224;71") {
		t.Fatalf("RenderMetadataSubline() should not include flashy yellow unstaged color, got %q", got)
	}
}

func TestRenderMetadataSublineColorsUnstagedIconBlueAndCountGreen(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Modified: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true})

	for _, want := range []string{"38;2;96;165;250", "38;2;74;222;128"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderMetadataSubline() should include color %s, got %q", want, got)
		}
	}
	if strings.Contains(got, "38;2;234;179;8") {
		t.Fatalf("RenderMetadataSubline() should not color unstaged metadata yellow, got %q", got)
	}
	if stripANSI(got) != " 2" {
		t.Fatalf("RenderMetadataSubline() = %q", got)
	}
}

func TestRenderMetadataSublineDesaturatesInactiveGitParts(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Selected: true, Active: false})

	for _, forbidden := range []string{"38;2;125;211;252", "38;2;134;239;172", "38;2;248;113;113", "38;2;147;197;253", "38;2;253;224;71", "38;2;96;165;250", "38;2;74;222;128"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("RenderMetadataSubline() should not include active color %s, got %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "38;2;75;85;99") {
		t.Fatalf("RenderMetadataSubline() should use inactive dark gray, got %q", got)
	}
	if stripANSI(got) != " 12 -2  3  8" {
		t.Fatalf("RenderMetadataSubline() = %q", got)
	}
}
