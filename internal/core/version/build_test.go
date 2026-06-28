package version

import "testing"

func TestBuildMetadataDisplay(t *testing.T) {
	tests := []struct {
		name string
		meta BuildMetadata
		want string
	}{
		{name: "release normalizes v prefix", meta: BuildMetadata{Kind: BuildKindRelease, Version: "0.23.1", Tag: "0.23.1", Commit: "abc123456789", HasDistance: true, Distance: 0}, want: "v0.23.1"},
		{name: "source at tag keeps commit", meta: BuildMetadata{Kind: BuildKindSource, Version: "v0.23.1", Tag: "v0.23.1", Commit: "abc123456789", HasDistance: true, Distance: 0}, want: "v0.23.1+0.gabc1234"},
		{name: "source ahead of tag", meta: BuildMetadata{Kind: BuildKindSource, Tag: "v0.23.1", Commit: "abc123456789", HasDistance: true, Distance: 3}, want: "v0.23.1+3.gabc1234"},
		{name: "dirty source", meta: BuildMetadata{Kind: BuildKindSource, Tag: "v0.23.1", Commit: "abc123456789", HasDistance: true, Distance: 3, Dirty: true, DirtyKnown: true}, want: "v0.23.1+3.gabc1234*"},
		{name: "source without tag", meta: BuildMetadata{Kind: BuildKindSource, Version: "dev", Commit: "abc123456789", Dirty: true, DirtyKnown: true}, want: "dev.gabc1234*"},
		{name: "source without tag clean", meta: BuildMetadata{Kind: BuildKindSource, Version: "dev", Commit: "abc123456789", Dirty: false, DirtyKnown: true}, want: "dev.gabc1234"},
		{name: "unknown source", meta: BuildMetadata{Kind: BuildKindSource, Version: "dev"}, want: "dev"},
		{name: "unknown build falls back to dev", meta: BuildMetadata{Kind: BuildKindUnknown}, want: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.meta.Display(); got != tt.want {
				t.Fatalf("Display() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMetadataDetailStringsPreserveUnknowns(t *testing.T) {
	meta := BuildMetadata{Commit: "unknown", Date: "unknown"}
	if meta.DetailCommit() != "unknown" || meta.DetailDate() != "unknown" || meta.DirtyString() != "unknown" {
		t.Fatalf("details = commit %q date %q dirty %q", meta.DetailCommit(), meta.DetailDate(), meta.DirtyString())
	}
	meta = BuildMetadata{Commit: "abc123", Date: "2026-06-28T00:00:00Z", DirtyKnown: true, Dirty: false}
	if meta.DetailCommit() != "abc123" || meta.DetailDate() != "2026-06-28T00:00:00Z" || meta.DirtyString() != "false" {
		t.Fatalf("details = commit %q date %q dirty %q", meta.DetailCommit(), meta.DetailDate(), meta.DirtyString())
	}
}

func TestBuildMetadataReleaseCheckVersion(t *testing.T) {
	if got := (BuildMetadata{Kind: BuildKindRelease, Version: "0.23.1"}).ReleaseCheckVersion(); got != "v0.23.1" {
		t.Fatalf("release check version = %q", got)
	}
	if got := (BuildMetadata{Kind: BuildKindRelease, Version: "dev", Tag: "v0.23.1"}).ReleaseCheckVersion(); got != "v0.23.1" {
		t.Fatalf("release check fallback version = %q", got)
	}
	if got := (BuildMetadata{Kind: BuildKindSource, Version: "v0.23.1", Tag: "v0.23.1", Commit: "abc123456789"}).ReleaseCheckVersion(); got != "" {
		t.Fatalf("source check version = %q, want empty", got)
	}
	if got := (BuildMetadata{Kind: BuildKindUnknown, Version: "v0.23.1"}).ReleaseCheckVersion(); got != "" {
		t.Fatalf("unknown check version = %q, want empty", got)
	}
}

func TestNormalizeReleaseVersion(t *testing.T) {
	tests := map[string]string{
		"0.23.1":   "v0.23.1",
		"v0.23.1":  "v0.23.1",
		" dev ":    "",
		"1.2":      "",
		"vv0.23.1": "",
		"(devel)":  "",
	}
	for raw, want := range tests {
		if got := NormalizeReleaseVersion(raw); got != want {
			t.Fatalf("NormalizeReleaseVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFromBuildSettings(t *testing.T) {
	fallback := BuildInfoFallback{Revision: "fedcba987654", Time: "2026-06-28T07:18:02Z", Modified: "true"}
	meta := FromBuildSettings("dev", "unknown", "unknown", "source", "", "", "", fallback)
	if meta.Commit != "fedcba987654" || meta.Date != "2026-06-28T07:18:02Z" || !meta.Dirty || !meta.DirtyKnown || meta.Display() != "dev.gfedcba9*" {
		t.Fatalf("fallback meta = %#v, display %q", meta, meta.Display())
	}

	release := FromBuildSettings("0.23.1", "abc123456789", "", "goreleaser", "", "", "", BuildInfoFallback{})
	if release.Kind != BuildKindRelease || release.Tag != "v0.23.1" || !release.HasDistance || release.Distance != 0 || release.Dirty || !release.DirtyKnown {
		t.Fatalf("release meta = %#v", release)
	}

	moduleRelease := FromBuildSettings("dev", "unknown", "unknown", "source", "", "", "", BuildInfoFallback{Version: "v0.23.1"})
	if moduleRelease.Kind != BuildKindRelease || moduleRelease.Display() != "v0.23.1" || moduleRelease.ReleaseCheckVersion() != "v0.23.1" {
		t.Fatalf("module release meta = %#v display=%q check=%q", moduleRelease, moduleRelease.Display(), moduleRelease.ReleaseCheckVersion())
	}

	develSource := FromBuildSettings("(devel)", "unknown", "unknown", "source", "", "", "", BuildInfoFallback{})
	if develSource.Kind != BuildKindSource || develSource.Display() != "dev" {
		t.Fatalf("devel source meta = %#v display=%q", develSource, develSource.Display())
	}
}
