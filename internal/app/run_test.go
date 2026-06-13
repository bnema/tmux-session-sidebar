package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

type recordingRouter struct {
	route Route
	err   error
}

func (r *recordingRouter) Handle(_ context.Context, route Route, _ io.Writer, _ io.Writer) error {
	r.route = route
	return r.err
}

func TestRunDispatchesCommands(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantRoute  string
		wantArgs   []string
		wantFlags  map[string]string
		wantStderr string
	}{
		{name: "daemon serve", args: []string{"daemon", "serve"}, wantExit: 0, wantRoute: "daemon/serve", wantArgs: []string{"daemon", "serve"}},
		{name: "daemon serve ui", args: []string{"daemon", "serve-ui"}, wantExit: 0, wantRoute: "daemon/serve-ui", wantArgs: []string{"daemon", "serve-ui"}},
		{name: "daemon ensure", args: []string{"daemon", "ensure"}, wantExit: 0, wantRoute: "daemon/ensure"},
		{name: "daemon bootstrap", args: []string{"daemon", "bootstrap"}, wantExit: 0, wantRoute: "daemon/bootstrap"},
		{name: "self update", args: []string{"self-update"}, wantExit: 0, wantRoute: "runtime/self-update"},
		{name: "hook client attached", args: []string{"hook", "client-attached", "--client=/dev/pts/40", "--session=alpha"}, wantExit: 0, wantRoute: "hook/client-attached", wantFlags: map[string]string{"client": "/dev/pts/40", "session": "alpha"}},
		{name: "hook detached", args: []string{"hook", "client-detached", "--client=/dev/pts/40", "--session=__floating-popup-1"}, wantExit: 0, wantRoute: "hook/client-detached", wantFlags: map[string]string{"client": "/dev/pts/40", "session": "__floating-popup-1"}},
		{name: "hook session changed", args: []string{"hook", "client-session-changed", "--client=/dev/pts/40", "--session="}, wantExit: 0, wantRoute: "hook/client-session-changed", wantFlags: map[string]string{"client": "/dev/pts/40", "session": ""}},
		{name: "hook client resized", args: []string{"hook", "client-resized", "--client", "%1"}, wantExit: 0, wantRoute: "hook/client-resized", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook window resized", args: []string{"hook", "window-resized", "--window", "@1"}, wantExit: 0, wantRoute: "hook/window-resized", wantFlags: map[string]string{"window": "@1"}},
		{name: "hook window layout changed", args: []string{"hook", "window-layout-changed", "--window", "@1"}, wantExit: 0, wantRoute: "hook/window-layout-changed", wantFlags: map[string]string{"window": "@1"}},
		{name: "hook agent event", args: []string{"hook", "agent-event", "--agent", "pi", "--event", "end", "--pane", "%2"}, wantExit: 0, wantRoute: "hook/agent-event", wantFlags: map[string]string{"agent": "pi", "event": "end", "pane": "%2"}},
		{name: "hooks setup", args: []string{"hooks", "setup", "--agent", "codex"}, wantExit: 0, wantRoute: "hooks/run", wantArgs: []string{"hooks", "setup"}, wantFlags: map[string]string{"agent": "codex"}},
		{name: "hooks agent event", args: []string{"hooks", "codex", "stop", "--pane", "%2"}, wantExit: 0, wantRoute: "hooks/run", wantArgs: []string{"hooks", "codex", "stop"}, wantFlags: map[string]string{"pane": "%2"}},
		{name: "resurrect post save layout", args: []string{"resurrect", "post-save-layout", "/tmp/with spaces/tmux_resurrect.txt"}, wantExit: 0, wantRoute: "resurrect/post-save-layout", wantArgs: []string{"/tmp/with spaces/tmux_resurrect.txt"}},
		{name: "sidebar toggle", args: []string{"sidebar", "toggle", "--client", "%1"}, wantExit: 0, wantRoute: "sidebar/toggle", wantFlags: map[string]string{"client": "%1"}},
		{name: "sidebar open", args: []string{"sidebar", "open", "--client=%1"}, wantExit: 0, wantRoute: "sidebar/open", wantFlags: map[string]string{"client": "%1"}},
		{name: "sidebar close", args: []string{"sidebar", "close", "--client", "%1"}, wantExit: 0, wantRoute: "sidebar/close", wantFlags: map[string]string{"client": "%1"}},
		{name: "action switch", args: []string{"action", "switch", "--client", "%1", "--session", "alpha"}, wantExit: 0, wantRoute: "action/switch", wantFlags: map[string]string{"client": "%1", "session": "alpha"}},
		{name: "action quick switch", args: []string{"action", "quick-switch", "--slot", "1"}, wantExit: 0, wantRoute: "action/quick-switch", wantFlags: map[string]string{"slot": "1"}},
		{name: "action create project", args: []string{"action", "create-project", "--client", "%1"}, wantExit: 0, wantRoute: "action/create-project", wantFlags: map[string]string{"client": "%1"}},
		{name: "action create current git project", args: []string{"action", "create-current-git-project", "--client", "%1"}, wantExit: 0, wantRoute: "action/create-current-git-project", wantFlags: map[string]string{"client": "%1"}},
		{name: "action create adhoc", args: []string{"action", "create-adhoc", "--name", "scratch"}, wantExit: 0, wantRoute: "action/create-adhoc", wantFlags: map[string]string{"name": "scratch"}},
		{name: "action rename", args: []string{"action", "rename", "--session", "old", "--name", "new"}, wantExit: 0, wantRoute: "action/rename", wantFlags: map[string]string{"session": "old", "name": "new"}},
		{name: "action kill", args: []string{"action", "kill", "--session", "old"}, wantExit: 0, wantRoute: "action/kill", wantFlags: map[string]string{"session": "old"}},
		{name: "action toggle numeric", args: []string{"action", "toggle-numeric", "--client", "%1"}, wantExit: 0, wantRoute: "action/toggle-numeric", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook agent event with pane-id and session-id", args: []string{"hook", "agent-event", "--pane-id", "%10", "--session-id", "$2"}, wantExit: 0, wantRoute: "hook/agent-event", wantFlags: map[string]string{"pane-id": "%10", "session-id": "$2"}},
		{name: "action create adhoc with source-path", args: []string{"action", "create-adhoc", "--source-path", "/home/user/project"}, wantExit: 0, wantRoute: "action/create-adhoc", wantFlags: map[string]string{"source-path": "/home/user/project"}},
		{name: "action create current git project with source-path and category-id", args: []string{"action", "create-current-git-project", "--source-path", "/tmp/myrepo", "--category-id", "cat1"}, wantExit: 0, wantRoute: "action/create-current-git-project", wantFlags: map[string]string{"source-path": "/tmp/myrepo", "category-id": "cat1"}},
		{name: "sidebar open with width and attach-target", args: []string{"sidebar", "open", "--client", "%1", "--width", "45", "--attach-target", "@1"}, wantExit: 0, wantRoute: "sidebar/open", wantFlags: map[string]string{"client": "%1", "width": "45", "attach-target": "@1"}},
		{name: "unknown command", args: []string{"nope"}, wantExit: 2},
		{name: "missing command shows help", args: nil, wantExit: 0},
		{name: "router error", args: []string{"daemon", "serve"}, wantExit: 1, wantRoute: "daemon/serve", wantArgs: []string{"daemon", "serve"}, wantStderr: "Error: boom\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			router := &recordingRouter{}
			if tt.name == "router error" {
				router.err = errors.New("boom")
			}

			exitCode := Run(context.Background(), tt.args, stdout, stderr, router)
			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d, want %d; stderr=%q", exitCode, tt.wantExit, stderr.String())
			}
			if router.route.Path != tt.wantRoute {
				t.Fatalf("route = %q, want %q", router.route.Path, tt.wantRoute)
			}
			if tt.wantArgs != nil && !slices.Equal(router.route.Args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", router.route.Args, tt.wantArgs)
			}
			if tt.wantStderr != "" && stderr.String() != tt.wantStderr {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			for key, want := range tt.wantFlags {
				if got := router.route.Flags[key]; got != want {
					t.Fatalf("flag %q = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestRunShowsVersion(t *testing.T) {
	oldVersion, oldCommit, oldDate, oldBuiltBy := version, commit, date, builtBy
	version, commit, date, builtBy = "1.2.3", "abc123", "2026-05-25T07:00:00Z", "test"
	defer func() { version, commit, date, builtBy = oldVersion, oldCommit, oldDate, oldBuiltBy }()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	router := &recordingRouter{}

	exitCode := Run(context.Background(), []string{"version"}, stdout, stderr, router)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	for _, want := range []string{"tmux-session-sidebar 1.2.3", "commit: abc123", "date: 2026-05-25T07:00:00Z", "builtBy: test"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if router.route.Path != "" {
		t.Fatalf("route = %q, want no dispatch", router.route.Path)
	}
}

func TestRunShowsHelp(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStdout []string
		wantStderr string
	}{
		{
			name:       "root help flag",
			args:       []string{"--help"},
			wantStdout: []string{"tmux-session-sidebar manages", "Available Commands:", "hooks"},
		},
		{
			name:       "root with no args",
			args:       nil,
			wantStdout: []string{"tmux-session-sidebar manages", "Available Commands:"},
		},
		{
			name:       "hook parent help",
			args:       []string{"hook"},
			wantStdout: []string{"Handle tmux runtime hooks", "client-attached", "agent-event"},
		},
		{
			name:       "hooks parent help",
			args:       []string{"hooks"},
			wantStdout: []string{"Install agent integrations", "Usage:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			router := &recordingRouter{}

			exitCode := Run(context.Background(), tt.args, stdout, stderr, router)
			if exitCode != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Fatalf("stderr = %q, want %q", got, tt.wantStderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			if router.route.Path != "" {
				t.Fatalf("route = %q, want no dispatch", router.route.Path)
			}
		})
	}
}
