package githubrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientLatestReleaseTagFetchesTagName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/bnema/tmux-session-sidebar/releases/latest" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept header = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Fatalf("X-GitHub-Api-Version header = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "tmux-session-sidebar" {
			t.Fatalf("User-Agent header = %q", got)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.10.3"}`))
	}))
	defer server.Close()

	client := Client{BaseURL: " " + server.URL + " ", HTTPClient: server.Client(), Timeout: time.Second}
	tag, err := client.LatestReleaseTag(context.Background())
	if err != nil {
		t.Fatalf("LatestReleaseTag returned error: %v", err)
	}
	if tag != "v0.10.3" {
		t.Fatalf("tag = %q, want v0.10.3", tag)
	}
}

func TestClientLatestReleaseTagReturnsErrorForNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client(), Timeout: time.Second}
	if _, err := client.LatestReleaseTag(context.Background()); err == nil {
		t.Fatal("LatestReleaseTag returned nil error for non-OK status")
	}
}

func TestClientLatestReleaseTagReturnsErrorForEmptyTagName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":""}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client(), Timeout: time.Second}
	if _, err := client.LatestReleaseTag(context.Background()); err == nil {
		t.Fatal("LatestReleaseTag returned nil error for empty tag_name")
	}
}

func TestClientLatestReleaseTagHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v0.10.3"}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client(), Timeout: time.Nanosecond}
	if _, err := client.LatestReleaseTag(context.Background()); err == nil {
		t.Fatal("LatestReleaseTag returned nil error for timed out request")
	}
}
