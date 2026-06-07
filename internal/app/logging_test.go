package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLogWriterRotatesBeforeWriteWouldExceedLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.log")
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	writer, err := newRotatingLogWriter(path, 10)
	if err != nil {
		t.Fatalf("newRotatingLogWriter error: %v", err)
	}
	defer writer.Close()
	if _, err := writer.Write([]byte("ab")); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if err := writer.Sync(); err != nil {
		t.Fatalf("Sync error: %v", err)
	}

	active, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	if got := string(active); got != "ab" {
		t.Fatalf("active log = %q, want fresh write only", got)
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if got := string(rotated); got != "123456789" {
		t.Fatalf("rotated log = %q, want old content", got)
	}
}

func TestRedirectStderrToRotatingLogRotatesDuringDaemonLifetime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "errors.log")
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	restore, err := redirectStderrToRotatingLog(path, 10)
	if err != nil {
		t.Fatalf("redirectStderrToRotatingLog error: %v", err)
	}
	fmt.Fprint(os.Stderr, "ab")
	restore()

	active, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	if got := strings.TrimSpace(string(active)); got != "ab" {
		t.Fatalf("active log = %q, want fresh stderr write only", got)
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if got := string(rotated); got != "123456789" {
		t.Fatalf("rotated log = %q, want old content", got)
	}
}
