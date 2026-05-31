package app

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSessionMetadataCapturePathPrefersLivePath(t *testing.T) {
	live := map[string]string{"ero": "/live/ero"}
	persisted := ports.SessionMetadata{ProjectPath: "/persisted/ero", LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, live)
	if !ok || got != "/live/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want live path", got, ok)
	}
}

func TestSessionMetadataCapturePathFallsBackToPersistedPath(t *testing.T) {
	persisted := ports.SessionMetadata{ProjectPath: "/persisted/ero", LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, nil)
	if !ok || got != "/persisted/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want persisted path", got, ok)
	}
}

func TestGitStatusMetadataSublineCarriesUpstreamMissing(t *testing.T) {
	got := gitStatusMetadataSubline(ports.GitStatus{Branch: "work", Clean: true, UpstreamConfigured: false})
	if !got.UpstreamMissing {
		t.Fatalf("UpstreamMissing = false, want true: %#v", got)
	}
}
