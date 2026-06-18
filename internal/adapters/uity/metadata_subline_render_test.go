package uity

import (
	"strings"
	"testing"
)

func TestRenderMetadataSublineColorsActiveGitParts(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Selected: true, Active: true})

	for _, want := range []string{"38;2;125;211;252", "38;2;147;197;253", "38;2;228;217;135"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderMetadataSubline() should include color %s, got %q", want, got)
		}
	}
	if strings.Contains(got, "38;2;253;224;71") {
		t.Fatalf("RenderMetadataSubline() should not include flashy yellow unstaged color, got %q", got)
	}
}

func TestRenderMetadataSublineColorsUnstagedWorktreeSoftAmber(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Modified: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true})

	if !strings.Contains(got, "38;2;214;200;111") {
		t.Fatalf("RenderMetadataSubline() should color unstaged worktree muted quince, got %q", got)
	}
	if strings.Contains(got, "38;2;234;179;8") || strings.Contains(got, "38;2;253;224;71") {
		t.Fatalf("RenderMetadataSubline() should not color unstaged metadata vivid yellow, got %q", got)
	}
	if stripANSI(got) != "*2" {
		t.Fatalf("RenderMetadataSubline() = %q", got)
	}
}

func TestRenderMetadataSublineKeepsASCIIUnstagedCountSinglePart(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Modified: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsASCII, Width: 80, Active: true})

	if stripANSI(got) != "U2" {
		t.Fatalf("RenderMetadataSubline() = %q, want ASCII unstaged count U2", got)
	}
}

func TestRenderMetadataSublineEllipsizesLongBranchBeforeStyling(t *testing.T) {
	branch := "feature/very-long-branch-name-that-would-overflow-during-switch"
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Branch: branch, Modified: 3}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 24, Selected: true, Active: true})
	plain := stripANSI(got)
	if strings.Contains(plain, branch) || !strings.Contains(plain, "…") {
		t.Fatalf("RenderMetadataSubline() = %q plain=%q, want pre-styled ellipsized branch", got, plain)
	}
	if width := metadataDisplayWidth(plain); width > 24 {
		t.Fatalf("RenderMetadataSubline() visible width = %d, want <= 24 (%q)", width, plain)
	}
}

func TestRenderMetadataSublineDesaturatesInactiveGitParts(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 12, Behind: 2, Staged: 3, Modified: 8}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: false})

	for _, forbidden := range []string{"38;2;125;211;252", "38;2;134;239;172", "38;2;248;113;113", "38;2;147;197;253", "38;2;253;224;71", "38;2;96;165;250", "38;2;74;222;128"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("RenderMetadataSubline() should not include active color %s, got %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "38;2;55;65;81") {
		t.Fatalf("RenderMetadataSubline() should use inactive metadata gray, got %q", got)
	}
	if stripANSI(got) != "⇄12/2 +3 *8" {
		t.Fatalf("RenderMetadataSubline() = %q", got)
	}
}

func TestRenderMetadataSublineSelectedInactiveUsesReadableSlate(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 1}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Selected: true, Active: false})
	if !strings.Contains(got, "38;2;148;163;184") {
		t.Fatalf("RenderMetadataSubline() should use selected inactive metadata slate, got %q", got)
	}
}

func TestRenderMetadataSublineUsesInactiveGradientShade(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 1}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: false, InactiveIntensity: 1})
	if !strings.Contains(got, "38;2;184;184;186") {
		t.Fatalf("RenderMetadataSubline() should use lighter inactive metadata gradient shade, got %q", got)
	}
	if strings.Contains(got, "38;2;204;204;204") {
		t.Fatalf("RenderMetadataSubline() should stay slightly darker than session gray, got %q", got)
	}
}

func TestRenderMetadataSublineSelectedInactiveIgnoresGradientIntensity(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 1}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Selected: true, Active: false, InactiveIntensity: 1})
	if !strings.Contains(got, "38;2;148;163;184") {
		t.Fatalf("RenderMetadataSubline() should keep selected inactive slate, got %q", got)
	}
	if strings.Contains(got, "38;2;184;184;186") {
		t.Fatalf("RenderMetadataSubline() should not keep inactive gradient when selected, got %q", got)
	}
}

func TestRenderMetadataSublineActiveIgnoresInactiveGradientIntensity(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Modified: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true, InactiveIntensity: 1})
	if !strings.Contains(got, "38;2;214;200;111") {
		t.Fatalf("RenderMetadataSubline() should keep active metadata colors, got %q", got)
	}
	if strings.Contains(got, "38;2;184;184;186") {
		t.Fatalf("RenderMetadataSubline() should not keep inactive gradient when active, got %q", got)
	}
}

func TestRenderMetadataSublineMutesDivergenceSlash(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 2, Behind: 1}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true})
	if stripANSI(got) != "⇄2/1" {
		t.Fatalf("RenderMetadataSubline() = %q", got)
	}
	if !strings.Contains(got, "38;2;100;116;139m/") {
		t.Fatalf("RenderMetadataSubline() should render divergence slash muted, got %q", got)
	}
}

func TestRenderMetadataSublineHidesZeroBehindDivergence(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Ahead: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true})
	if stripANSI(got) != "⇄2" {
		t.Fatalf("RenderMetadataSubline() = %q, want compact one-sided divergence", got)
	}
	if strings.Contains(stripANSI(got), "/0") {
		t.Fatalf("RenderMetadataSubline() should hide zero behind side, got %q", got)
	}
}

func TestRenderMetadataSublineCombinesUpstreamPushPull(t *testing.T) {
	got := RenderMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, UpstreamAhead: 1, UpstreamBehind: 2}, MetadataSublineRenderOptions{Icons: MetadataIconsNerd, Width: 80, Active: true})
	if stripANSI(got) != "↑1↓2" {
		t.Fatalf("RenderMetadataSubline() = %q, want combined upstream push/pull", got)
	}
	if !strings.Contains(got, "38;2;74;222;128m↑1") || !strings.Contains(got, "38;2;248;113;113m↓2") {
		t.Fatalf("RenderMetadataSubline() should color upstream sides separately, got %q", got)
	}
}
