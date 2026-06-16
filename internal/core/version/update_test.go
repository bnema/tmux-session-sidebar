package version

import "testing"

func TestUpdateAvailableReportsDevBuildsAsUpdateAvailable(t *testing.T) {
	if !UpdateAvailable("dev", "") {
		t.Fatal("dev builds should always show the update indicator")
	}
}

func TestCheckableReleaseVersionRejectsDevAndInvalidVersions(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "dev", want: false},
		{version: "source", want: false},
		{version: "", want: false},
		{version: "0.10.2", want: true},
		{version: "v0.10.2", want: true},
		{version: "v0.10.-2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := CheckableReleaseVersion(tt.version); got != tt.want {
				t.Fatalf("CheckableReleaseVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestUpdateAvailableComparesSemanticTags(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "newer patch", current: "v0.10.2", latest: "v0.10.3", want: true},
		{name: "same version", current: "0.10.3", latest: "v0.10.3", want: false},
		{name: "older latest", current: "v0.10.4", latest: "v0.10.3", want: false},
		{name: "newer minor", current: "v0.10.9", latest: "v0.11.0", want: true},
		{name: "invalid current", current: "source", latest: "v0.11.0", want: false},
		{name: "invalid latest", current: "v0.10.2", latest: "latest", want: false},
		{name: "negative latest", current: "v0.10.2", latest: "v0.10.-3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdateAvailable(tt.current, tt.latest); got != tt.want {
				t.Fatalf("UpdateAvailable(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
