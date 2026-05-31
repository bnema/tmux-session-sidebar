package uity

import "testing"

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

	want := " 2 -1  1  3  1"
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

	want := "2 -1 +1 ~3 ?1"
	if got != want {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, want)
	}
}

func TestFormatMetadataSublineOmitsCleanGitStates(t *testing.T) {
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
			if got != "" {
				t.Fatalf("FormatMetadataSubline() = %q, want empty", got)
			}
		})
	}
}

func TestFormatMetadataSublineOmitsCleanGitWithoutUpstream(t *testing.T) {
	got := FormatMetadataSubline(SessionMetadataSubline{Kind: MetadataKindGit, Branch: "work", Clean: true, UpstreamMissing: true}, MetadataSublineOptions{Icons: MetadataIconsNerd, Width: 80})
	if got != "" {
		t.Fatalf("FormatMetadataSubline() = %q, want empty", got)
	}
}

func TestFormatMetadataSublineOmitsBranchEvenWhenLong(t *testing.T) {
	meta := SessionMetadataSubline{
		Kind:     MetadataKindGit,
		Branch:   "feature/add-session-metadata-subline",
		Ahead:    2,
		Modified: 3,
	}

	got := FormatMetadataSubline(meta, MetadataSublineOptions{Icons: MetadataIconsNerd, Width: 24})
	want := " 2  3"
	if got != want {
		t.Fatalf("FormatMetadataSubline() = %q, want %q", got, want)
	}
	if width := metadataDisplayWidth(got); width > 24 {
		t.Fatalf("FormatMetadataSubline() width = %d, want <= 24 (%q)", width, got)
	}
}

func TestFormatMetadataSublineFallsBackToDirtySummaryWhenWidthIsTight(t *testing.T) {
	meta := SessionMetadataSubline{
		Kind:      MetadataKindGit,
		Branch:    "feature/add-session-metadata-subline",
		Ahead:     2,
		Behind:    1,
		Staged:    1,
		Modified:  3,
		Untracked: 1,
	}

	got := FormatMetadataSubline(meta, MetadataSublineOptions{Icons: MetadataIconsASCII, Width: 12})
	want := "dirty5"
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
