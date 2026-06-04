package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfUpdateRunsPluginUpdaterBesideRuntimeBinary(t *testing.T) {
	pluginDir := t.TempDir()
	binDir := filepath.Join(pluginDir, ".bin")
	scriptsDir := filepath.Join(pluginDir, "scripts")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	runtimePath := filepath.Join(binDir, "tmux-session-sidebar")
	if err := os.WriteFile(runtimePath, []byte("runtime"), 0o755); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	updaterPath := filepath.Join(scriptsDir, "update-runtime.sh")
	logPath := filepath.Join(pluginDir, "update.log")
	updater := "#!/usr/bin/env bash\n" +
		"printf 'cwd=%s\\n' \"$PWD\" >>\"$TEST_UPDATE_LOG\"\n" +
		"printf 'release_only=%s\\n' \"${TMUX_SESSION_SIDEBAR_RELEASE_ONLY:-}\" >>\"$TEST_UPDATE_LOG\"\n" +
		"printf 'build_from_source=%s\\n' \"${TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE:-}\" >>\"$TEST_UPDATE_LOG\"\n" +
		"printf 'stdout from updater\\n'\n" +
		"printf 'stderr from updater\\n' >&2\n"
	if err := os.WriteFile(updaterPath, []byte(updater), 0o755); err != nil {
		t.Fatalf("write updater: %v", err)
	}
	t.Setenv("TEST_UPDATE_LOG", logPath)
	oldExecutablePath := selfUpdateExecutablePath
	selfUpdateExecutablePath = func() (string, error) { return runtimePath, nil }
	defer func() { selfUpdateExecutablePath = oldExecutablePath }()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "runtime/self-update"}, stdout, stderr); err != nil {
		t.Fatalf("self-update error: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read updater log: %v", err)
	}
	wantLog := "cwd=" + pluginDir + "\nrelease_only=1\nbuild_from_source="
	if got := strings.TrimSpace(string(logBytes)); got != wantLog {
		t.Fatalf("updater log = %q, want %q", got, wantLog)
	}
	if !strings.Contains(stdout.String(), "stdout from updater") {
		t.Fatalf("stdout missing updater output: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr from updater") {
		t.Fatalf("stderr missing updater output: %q", stderr.String())
	}
}

func TestSelfUpdateReportsMissingPluginUpdater(t *testing.T) {
	pluginDir := t.TempDir()
	binDir := filepath.Join(pluginDir, ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	runtimePath := filepath.Join(binDir, "tmux-session-sidebar")
	if err := os.WriteFile(runtimePath, []byte("runtime"), 0o755); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	oldExecutablePath := selfUpdateExecutablePath
	selfUpdateExecutablePath = func() (string, error) { return runtimePath, nil }
	defer func() { selfUpdateExecutablePath = oldExecutablePath }()

	err := (runtimeRouter{}).Handle(context.Background(), Route{Path: "runtime/self-update"}, nil, nil)
	if err == nil {
		t.Fatal("self-update error = nil, want missing updater error")
	}
	want := filepath.Join(pluginDir, "scripts", "update-runtime.sh")
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want missing updater path %q", err.Error(), want)
	}
}
