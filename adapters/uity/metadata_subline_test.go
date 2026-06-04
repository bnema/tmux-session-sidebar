package uity

import (
	"strings"
	"testing"
)

func TestFormatMetadataSublineNerdGitStates(t *testing.T) {
	got := FormatMetadataSubline(SessionMetadataSubline{
		Kind:      MetadataKindGit,
		Branch:    "feat/ui",
		Ahead:     2,
		Behind:    1,
		Staged:    1,
		Modified:  3,
		Untracked: 1,
	}, MetadataSublineOptions{Icons: MetadataIconsNerd, Width: 80})

	want := "  feat/ui  2 -1  1  4"
	if got != want {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, want)
	}
}

func TestFormatMetadataSublineASCIIGitStates(t *testing.T) {
	got := FormatMetadataSubline(SessionMetadataSubline{
		Kind:      MetadataKindGit,
		Branch:    "feat/ui",
		Ahead:     2,
		Behind:    1,
		Staged:    1,
		Modified:  3,
		Untracked: 1,
	}, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 80})

	want := "git feat/ui 2 -1 S1 U4"
	if got != want {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, want)
	}
}

func TestFormatMetadataSublineLoadingUsesAsciiEllipsisInAsciiMode(t *testing.T) {
	got := FormatMetadataSubline(SessionMetadataSubline{Kind: MetadataKindLoading}, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 10})
	if got != "..." {
		t.Fatalf("ASCII loading metadata = %q, want ...", got)
	}
}

func TestFormatMetadataSublineShowsCleanGitBranch(t *testing.T) {
	tests := []struct {
		name  string
		icons MetadataIconMode
	}{
		{name: "nerd", icons: MetadataIconsNerd},
		{name: "ascii", icons: MetadataIconsASCII},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Branch: "main", Clean: true}, MetadataSublineOptions{Icons: tt.icons, Width: 80})
			if got == "" || strings.Contains(got, "clean") {
				t.Fatalf("FormatMetadataSubline() = %q, want branch without clean suffix", got)
			}
		})
	}
}

func TestFormatMetadataSublineShowsCleanGitWithoutUpstream(t *testing.T) {
	got := FormatMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Branch: "work", Clean: true, UpstreamMissing: true}, MetadataSublineOptions{Icons: MetadataIconsNerd, Width: 80})
	if got != "  work" {
		t.Fatalf("FormatMetadataSubline() = %q, want branch with clean state", got)
	}
}

func TestFormatMetadataSublineEllipsizesLongBranch(t *testing.T) {
	meta := SessionMetadataSubline{
		Kind:     MetadataKindGit,
		Branch:   "feature/add-session-metadata-subline",
		Ahead:    2,
		Modified: 3,
	}

	got := FormatMetadataSubline(meta, MetadataSublineOptions{Icons: MetadataIconsNerd, Width: 24})
	for _, want := range []string{"  feature", "…", " 2", " 3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatMetadataSubline() = %q, want to contain %q", got, want)
		}
	}
	if width := metadataDisplayWidth(got); width > 24 {
		t.Fatalf("FormatMetadataSubline() width = %d, want <= 24 (%q)", width, got)
	}
}

func TestFormatMetadataSublineFallsBackToUnstagedSummaryWhenWidthIsTight(t *testing.T) {
	meta := SessionMetadataSubline{
		Kind:      MetadataKindGit,
		Branch:    "feature/add-session-metadata-subline",
		Ahead:     2,
		Behind:    1,
		Staged:    1,
		Modified:  3,
		Untracked: 1,
	}

	got := FormatMetadataSubline(meta, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 8})
	want := "2 -1 U4"
	if got != want {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, want)
	}
	if width := metadataDisplayWidth(got); width > 12 {
		t.Fatalf("FormatMetadataSubline() width = %d, want <= 12 (%q)", width, got)
	}
}

func TestFormatMetadataSublineNonGitContextDoesNotRepeatSessionName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := FormatMetadataSubline(SessionMetadataSubline{
		Kind:        MetadataKindDirectory,
		SessionName: "notes",
		Path:        home + "/Notes",
	}, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 20})
	if got != "dir ~/Notes" {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, "dir ~/Notes")
	}

	got = FormatMetadataSubline(SessionMetadataSubline{
		Kind:        MetadataKindDirectory,
		SessionName: "notes",
		Path:        "/home/example/notes",
	}, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 20})
	if got != "" {
		t.Fatalf("FormatMetadataSubline() repeated session name: %q", got)
	}
}
