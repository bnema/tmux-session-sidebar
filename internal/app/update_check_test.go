package app

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestCheckUpdateAvailableComparesCurrentVersionWithLatestRelease(t *testing.T) {
	checker := mocks.NewMockReleaseCheckerPort(t)
	checker.EXPECT().LatestReleaseTag(mock.Anything).Return("v0.10.3", nil)

	check := newUpdateAvailableCheck(context.Background(), checker)
	available, err := check("0.10.2")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if !available {
		t.Fatal("available = false, want true")
	}
}

func TestCheckUpdateAvailablePropagatesReleaseErrors(t *testing.T) {
	checker := mocks.NewMockReleaseCheckerPort(t)
	checker.EXPECT().LatestReleaseTag(mock.Anything).Return("", errors.New("network failed"))

	check := newUpdateAvailableCheck(context.Background(), checker)
	available, err := check("0.10.2")
	if err == nil {
		t.Fatal("check returned nil error")
	}
	if available {
		t.Fatal("available = true, want false on error")
	}
}

func TestCheckUpdateAvailableUsesProvidedContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "sidebar")
	checker := mocks.NewMockReleaseCheckerPort(t)
	checker.EXPECT().LatestReleaseTag(mock.MatchedBy(func(got context.Context) bool {
		return got.Value(contextKey("request")) == "sidebar"
	})).Return("v0.10.2", nil)

	check := newUpdateAvailableCheck(ctx, checker)
	if _, err := check("0.10.2"); err != nil {
		t.Fatalf("check returned error: %v", err)
	}
}
