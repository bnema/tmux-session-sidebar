package projects

import "testing"

func TestNormalizeSessionName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple lowercase", in: "alpha", want: "alpha"},
		{name: "uppercase and space", in: "My Repo", want: "my_repo"},
		{name: "already clean trims underscores", in: "__already__clean__", want: "already_clean"},
		{name: "dots fallback", in: "...", want: "session"},
		{name: "unicode separator", in: "été", want: "t"},
		{name: "collapses punctuation", in: "my   repo!!!", want: "my_repo"},
		{name: "collapses hyphens", in: "my---repo", want: "my-repo"},
		{name: "trims mixed edges", in: "-_repo_.", want: "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeSessionName(tt.in); got != tt.want {
				t.Fatalf("NormalizeSessionName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCandidateFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Candidate
	}{
		{name: "repo basename", path: "/tmp/My Repo", want: Candidate{Path: "/tmp/My Repo", SessionName: "my_repo"}},
		{name: "root fallback", path: "/", want: Candidate{Path: "/", SessionName: "session"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CandidateFromPath(tt.path)
			if got != tt.want {
				t.Fatalf("CandidateFromPath(%q) = %#v, want %#v", tt.path, got, tt.want)
			}
		})
	}
}
