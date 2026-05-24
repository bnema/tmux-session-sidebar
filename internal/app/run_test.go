package app

import (
	"bytes"
	"context"
	"errors"
	"io"
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
		name      string
		args      []string
		wantExit  int
		wantRoute string
		wantFlags map[string]string
	}{
		{name: "daemon serve", args: []string{"daemon", "serve"}, wantExit: 0, wantRoute: "daemon/serve"},
		{name: "daemon ensure", args: []string{"daemon", "ensure"}, wantExit: 0, wantRoute: "daemon/ensure"},
		{name: "hook client attached", args: []string{"hook", "client-attached", "--client", "%1"}, wantExit: 0, wantRoute: "hook/client-attached", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook detached", args: []string{"hook", "client-detached", "--client", "%1"}, wantExit: 0, wantRoute: "hook/client-detached", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook session changed", args: []string{"hook", "client-session-changed", "--client", "%1"}, wantExit: 0, wantRoute: "hook/client-session-changed", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook client resized", args: []string{"hook", "client-resized", "--client", "%1"}, wantExit: 0, wantRoute: "hook/client-resized", wantFlags: map[string]string{"client": "%1"}},
		{name: "hook window resized", args: []string{"hook", "window-resized", "--window", "@1"}, wantExit: 0, wantRoute: "hook/window-resized", wantFlags: map[string]string{"window": "@1"}},
		{name: "hook agent event", args: []string{"hook", "agent-event", "--agent", "pi", "--event", "end", "--pane", "%2"}, wantExit: 0, wantRoute: "hook/agent-event", wantFlags: map[string]string{"agent": "pi", "event": "end", "pane": "%2"}},
		{name: "hooks setup", args: []string{"hooks", "setup", "--agent", "codex"}, wantExit: 0, wantRoute: "hooks/run", wantFlags: map[string]string{"agent": "codex"}},
		{name: "hooks agent event", args: []string{"hooks", "codex", "stop", "--pane", "%2"}, wantExit: 0, wantRoute: "hooks/run", wantFlags: map[string]string{"pane": "%2"}},
		{name: "sidebar toggle", args: []string{"sidebar", "toggle", "--client", "%1"}, wantExit: 0, wantRoute: "sidebar/toggle", wantFlags: map[string]string{"client": "%1"}},
		{name: "sidebar open", args: []string{"sidebar", "open", "--client=%1"}, wantExit: 0, wantRoute: "sidebar/open", wantFlags: map[string]string{"client": "%1"}},
		{name: "sidebar close", args: []string{"sidebar", "close", "--client", "%1"}, wantExit: 0, wantRoute: "sidebar/close", wantFlags: map[string]string{"client": "%1"}},
		{name: "ui run", args: []string{"ui", "run", "--client", "%1", "--pane", "%9"}, wantExit: 0, wantRoute: "ui/run", wantFlags: map[string]string{"client": "%1", "pane": "%9"}},
		{name: "action switch", args: []string{"action", "switch", "--client", "%1", "--session", "alpha"}, wantExit: 0, wantRoute: "action/switch", wantFlags: map[string]string{"client": "%1", "session": "alpha"}},
		{name: "action quick switch", args: []string{"action", "quick-switch", "--slot", "1"}, wantExit: 0, wantRoute: "action/quick-switch", wantFlags: map[string]string{"slot": "1"}},
		{name: "action create project", args: []string{"action", "create-project", "--client", "%1"}, wantExit: 0, wantRoute: "action/create-project", wantFlags: map[string]string{"client": "%1"}},
		{name: "action create current git project", args: []string{"action", "create-current-git-project", "--client", "%1"}, wantExit: 0, wantRoute: "action/create-current-git-project", wantFlags: map[string]string{"client": "%1"}},
		{name: "action create adhoc", args: []string{"action", "create-adhoc", "--name", "scratch"}, wantExit: 0, wantRoute: "action/create-adhoc", wantFlags: map[string]string{"name": "scratch"}},
		{name: "action rename", args: []string{"action", "rename", "--session", "old", "--name", "new"}, wantExit: 0, wantRoute: "action/rename", wantFlags: map[string]string{"session": "old", "name": "new"}},
		{name: "action kill", args: []string{"action", "kill", "--session", "old"}, wantExit: 0, wantRoute: "action/kill", wantFlags: map[string]string{"session": "old"}},
		{name: "action toggle numeric", args: []string{"action", "toggle-numeric", "--client", "%1"}, wantExit: 0, wantRoute: "action/toggle-numeric", wantFlags: map[string]string{"client": "%1"}},
		{name: "unknown command", args: []string{"nope"}, wantExit: 2},
		{name: "missing command", args: nil, wantExit: 2},
		{name: "router error", args: []string{"daemon", "serve"}, wantExit: 1, wantRoute: "daemon/serve"},
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
			for key, want := range tt.wantFlags {
				if got := router.route.Flags[key]; got != want {
					t.Fatalf("flag %q = %q, want %q", key, got, want)
				}
			}
		})
	}
}
