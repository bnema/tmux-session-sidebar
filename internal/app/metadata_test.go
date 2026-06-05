package app

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestSessionMetadataCapturePathPrefersLivePathForCapturedSession(t *testing.T) {
	live := map[string]string{"ero": "/live/ero"}
	persisted := ports.SessionMetadata{Kind: "captured", LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, live)
	if !ok || got != "/live/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want live path", got, ok)
	}
}

func TestSessionMetadataCapturePathPrefersProjectPathOverLivePath(t *testing.T) {
	live := map[string]string{"ero": "/home/user/Downloads"}
	persisted := ports.SessionMetadata{Kind: "project", ProjectPath: "/persisted/ero", LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, live)
	if !ok || got != "/persisted/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want project path", got, ok)
	}
}

func TestSessionMetadataCapturePathPrefersLivePathForNonProjectWithStaleProjectPath(t *testing.T) {
	live := map[string]string{"ero": "/live/ero"}
	persisted := ports.SessionMetadata{Kind: "captured", ProjectPath: "/stale/project", LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, live)
	if !ok || got != "/live/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want live path", got, ok)
	}
}

func TestSessionMetadataCapturePathFallsBackToPersistedPath(t *testing.T) {
	persisted := ports.SessionMetadata{LastPath: "/persisted/ero"}
	got, ok := sessionMetadataCapturePath("ero", persisted, nil)
	if !ok || got != "/persisted/ero" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want persisted path", got, ok)
	}
}

func TestSessionMetadataCapturePathFallsBackToLastPathForNonProjectWithStaleProjectPath(t *testing.T) {
	persisted := ports.SessionMetadata{Kind: "adhoc", ProjectPath: "/stale/project", LastPath: "/last/path"}
	got, ok := sessionMetadataCapturePath("ero", persisted, nil)
	if !ok || got != "/last/path" {
		t.Fatalf("sessionMetadataCapturePath() = %q, %v; want last path", got, ok)
	}
}

func TestGitStatusMetadataSublineCarriesUpstreamMissing(t *testing.T) {
	got := gitStatusMetadataSubline(ports.GitStatus{Branch: "work", Clean: true, UpstreamConfigured: false})
	if !got.UpstreamMissing {
		t.Fatalf("UpstreamMissing = false, want true: %#v", got)
	}
}
