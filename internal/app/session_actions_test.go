package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func installFakeTmux(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(dir, "tmux.log")
	t.Setenv("TMUX_LOG", logPath)
	return logPath
}

func TestKillSessionConfirmationTargetsClient(t *testing.T) {
	tests := []struct {
		name         string
		client       string
		wantContains []string
	}{
		{name: "targets client and carries client to confirmed command", client: "/dev/pts/99", wantContains: []string{"confirm-before -t /dev/pts/99 -p Kill session alpha?", "--client", "/dev/pts/99"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

			err := killSession(t.Context(), map[string]string{"client": tt.client, "session": "alpha"}, nil)
			if err != nil {
				t.Fatalf("killSession returned error: %v", err)
			}
			log := readLog(t, logPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(log, want) {
					t.Fatalf("expected log to contain %q, log=%q", want, log)
				}
			}
		})
	}
}

func TestCommandPromptsUseQuotedTmuxInputPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		act  func(context.Context) error
	}{
		{name: "rename prompt", act: func(ctx context.Context) error {
			return renameSession(ctx, map[string]string{"client": "/dev/pts/99", "session": "alpha"}, nil)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
`)

			if err := tt.act(t.Context()); err != nil {
				t.Fatalf("prompt action error: %v", err)
			}
			log := readLog(t, logPath)
			if !strings.Contains(log, "--name") || !strings.Contains(log, "%%") {
				t.Fatalf("expected quoted tmux input placeholder, log=%q", log)
			}
			if strings.Contains(log, "%%%'") || strings.Contains(log, "%%\\%") {
				t.Fatalf("unexpected escaped placeholder, log=%q", log)
			}
		})
	}
}

func TestCreateAdhocUsesCurrentDirectoryNameWithoutPrompt(t *testing.T) {
	logPath := installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message)
    printf '/tmp/worktree/scratch\n'
    ;;
  list-sessions)
    ;;
esac
`)

	if err := createAdhoc(t.Context(), map[string]string{}, nil); err != nil {
		t.Fatalf("createAdhoc returned error: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "command-prompt") {
		t.Fatalf("expected no ad-hoc prompt, log=%q", log)
	}
	for _, want := range []string{"display-message -p #{pane_current_path}", "new-session -d -s scratch -c /tmp/worktree/scratch", "switch-client -t scratch"} {
		if !strings.Contains(log, want) {
			t.Fatalf("expected log to contain %q, log=%q", want, log)
		}
	}
}

func TestCreateAdhocPersistsRestoreMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message) printf '/tmp/worktree/scratch\n' ;;
  list-sessions) ;;
  set-option) ;;
esac
`)

	if err := createAdhoc(t.Context(), map[string]string{}, nil); err != nil {
		t.Fatalf("createAdhoc returned error: %v", err)
	}
	assertPersistedMetadata(t, "scratch", ports.SessionMetadata{Kind: "adhoc", LastPath: "/tmp/worktree/scratch"})
}

func TestCreateProjectPersistsRestoreMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) ;;
  set-option) ;;
esac
`)

	path := "/tmp/worktree/project"
	if err := createProject(t.Context(), map[string]string{"project-path": path}, nil, nil); err != nil {
		t.Fatalf("createProject returned error: %v", err)
	}
	assertPersistedMetadata(t, "project", ports.SessionMetadata{Kind: "project", ProjectPath: path, LastPath: path})
}

func TestCreateAdhocFailureRestoresPreviousMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	previous := ports.SessionMetadata{Kind: "captured", LastPath: "/tmp/old"}
	seedPersistedState(t, map[string]ports.SessionMetadata{"scratch": previous}, []string{"alpha", "scratch", "gamma"})
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message) printf '/tmp/worktree/scratch\n' ;;
  list-sessions) ;;
  new-session) exit 1 ;;
esac
`)

	if err := createAdhoc(t.Context(), map[string]string{}, nil); err == nil {
		t.Fatal("createAdhoc returned nil, want error")
	}
	assertPersistedMetadata(t, "scratch", previous)
	assertPersistedOrder(t, []string{"alpha", "scratch", "gamma"})
}

func TestCreateAdhocSwitchFailureKeepsNewMetadataWhenSessionExists(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	counter := filepath.Join(t.TempDir(), "list-count")
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  display-message) printf '/tmp/worktree/scratch\n' ;;
  list-sessions)
    count=0
    if [[ -f "`+counter+`" ]]; then count=$(cat "`+counter+`"); fi
    count=$((count + 1))
    printf '%s' "$count" > "`+counter+`"
    if [[ "$count" -ge 2 ]]; then printf '$9\tscratch\t1\t0\n'; fi
    ;;
  switch-client) exit 1 ;;
  set-option) ;;
esac
`)

	if err := createAdhoc(t.Context(), map[string]string{}, nil); err == nil {
		t.Fatal("createAdhoc returned nil, want switch error")
	}
	assertPersistedMetadata(t, "scratch", ports.SessionMetadata{Kind: "adhoc", LastPath: "/tmp/worktree/scratch"})
}

func TestCreateProjectFailureRestoresPreviousMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	previous := ports.SessionMetadata{Kind: "captured", LastPath: "/tmp/old-project"}
	seedPersistedState(t, map[string]ports.SessionMetadata{"project": previous}, []string{"alpha", "project", "gamma"})
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) ;;
  new-session) exit 1 ;;
esac
`)

	if err := createProject(t.Context(), map[string]string{"project-path": "/tmp/worktree/project"}, nil, nil); err == nil {
		t.Fatal("createProject returned nil, want error")
	}
	assertPersistedMetadata(t, "project", previous)
	assertPersistedOrder(t, []string{"alpha", "project", "gamma"})
}

func TestCreateProjectSwitchFailureKeepsNewMetadataWhenSessionExists(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	counter := filepath.Join(t.TempDir(), "list-count")
	path := "/tmp/worktree/project"
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions)
    count=0
    if [[ -f "`+counter+`" ]]; then count=$(cat "`+counter+`"); fi
    count=$((count + 1))
    printf '%s' "$count" > "`+counter+`"
    if [[ "$count" -ge 2 ]]; then printf '$1\tproject\t1\t0\n'; fi
    ;;
  switch-client) exit 1 ;;
  set-option) ;;
esac
`)

	if err := createProject(t.Context(), map[string]string{"project-path": path}, nil, nil); err == nil {
		t.Fatal("createProject returned nil, want switch error")
	}
	assertPersistedMetadata(t, "project", ports.SessionMetadata{Kind: "project", ProjectPath: path, LastPath: path})
}

func TestRenameSessionMovesMetadataAndRollsBackOnFailure(t *testing.T) {
	tests := []struct {
		name         string
		newName      string
		renameExit   string
		initial      map[string]ports.SessionMetadata
		initialOrder []string
		wantSessions map[string]ports.SessionMetadata
		wantOrder    []string
		wantErr      bool
	}{
		{
			name:         "success",
			newName:      "beta",
			renameExit:   "0",
			initial:      map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			initialOrder: []string{"alpha", "gamma"},
			wantSessions: map[string]ports.SessionMetadata{"beta": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			wantOrder:    []string{"beta", "gamma"},
		},
		{
			name:         "failure restores alpha",
			newName:      "beta",
			renameExit:   "1",
			initial:      map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			initialOrder: []string{"alpha", "gamma"},
			wantSessions: map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			wantOrder:    []string{"alpha", "gamma"},
			wantErr:      true,
		},
		{
			name:       "failure restores overwritten target metadata",
			newName:    "beta",
			renameExit: "1",
			initial: map[string]ports.SessionMetadata{
				"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"},
				"beta":  {Kind: "project", ProjectPath: "/tmp/beta", LastPath: "/tmp/beta"},
			},
			initialOrder: []string{"alpha", "beta", "gamma"},
			wantSessions: map[string]ports.SessionMetadata{
				"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"},
				"beta":  {Kind: "project", ProjectPath: "/tmp/beta", LastPath: "/tmp/beta"},
			},
			wantOrder: []string{"alpha", "beta", "gamma"},
			wantErr:   true,
		},
		{
			name:         "success to numeric removes restore record",
			newName:      "123",
			renameExit:   "0",
			initial:      map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			initialOrder: []string{"alpha", "gamma"},
			wantSessions: map[string]ports.SessionMetadata{},
			wantOrder:    []string{"gamma"},
		},
		{
			name:         "failure to hidden restores old record",
			newName:      "__hidden",
			renameExit:   "1",
			initial:      map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			initialOrder: []string{"alpha", "gamma"},
			wantSessions: map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}},
			wantOrder:    []string{"alpha", "gamma"},
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			seedPersistedState(t, tt.initial, tt.initialOrder)
			installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t1\n$2\tgamma\t1\t0\n' ;;
  rename-session) exit `+tt.renameExit+` ;;
esac
`)
			sidebar := mocks.NewMockTmuxSidebarPort(t)
			if !tt.wantErr {
				sidebar.EXPECT().RefreshSidebar(t.Context(), "").Return(nil)
			}
			err := renameSession(t.Context(), map[string]string{"session": "alpha", "name": tt.newName}, sidebar)
			if (err != nil) != tt.wantErr {
				t.Fatalf("renameSession error = %v, wantErr %v", err, tt.wantErr)
			}
			state, err := loadSidebarState(t.Context())
			if err != nil {
				t.Fatalf("loadSidebarState error = %v", err)
			}
			if !reflect.DeepEqual(state.Sessions, tt.wantSessions) {
				t.Fatalf("Sessions = %#v, want %#v", state.Sessions, tt.wantSessions)
			}
			if !reflect.DeepEqual(state.SessionOrder, tt.wantOrder) {
				t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, tt.wantOrder)
			}
		})
	}
}

func TestConfirmedKillRemovesPersistedMetadataAndOrder(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	seedPersistedState(t, map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}, "beta": {Kind: "adhoc", LastPath: "/tmp/beta"}}, []string{"alpha", "beta"})
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n' ;;
esac
`)
	sidebar := mocks.NewMockTmuxSidebarPort(t)
	sidebar.EXPECT().RefreshSidebar(t.Context(), "").Return(nil)

	if err := killSession(t.Context(), map[string]string{"session": "alpha", "confirmed": "yes"}, sidebar); err != nil {
		t.Fatalf("killSession returned error: %v", err)
	}
	state, err := loadSidebarState(t.Context())
	if err != nil {
		t.Fatalf("loadSidebarState error = %v", err)
	}
	if _, ok := state.Sessions["alpha"]; ok {
		t.Fatalf("Sessions[alpha] exists after kill: %#v", state.Sessions)
	}
	if want := []string{"beta"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, want)
	}
}

func TestConfirmedKillFailureRestoresPersistedMetadataAndOrder(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	initialSessions := map[string]ports.SessionMetadata{"alpha": {Kind: "adhoc", LastPath: "/tmp/alpha"}, "beta": {Kind: "adhoc", LastPath: "/tmp/beta"}}
	initialOrder := []string{"alpha", "beta"}
	seedPersistedState(t, initialSessions, initialOrder)
	installFakeTmux(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions) printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n' ;;
  kill-session) exit 1 ;;
esac
`)

	if err := killSession(t.Context(), map[string]string{"session": "alpha", "confirmed": "yes"}, nil); err == nil {
		t.Fatal("killSession returned nil, want error")
	}
	state, err := loadSidebarState(t.Context())
	if err != nil {
		t.Fatalf("loadSidebarState error = %v", err)
	}
	if !reflect.DeepEqual(state.Sessions, initialSessions) {
		t.Fatalf("Sessions = %#v, want %#v", state.Sessions, initialSessions)
	}
	if !reflect.DeepEqual(state.SessionOrder, initialOrder) {
		t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, initialOrder)
	}
}

func TestConfirmedKill(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		wantContains []string
	}{
		{
			name: "refreshes sidebar pane",
			script: `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions)
    printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n'
    ;;
  display-message)
    printf '%%1\n'
    ;;
  list-panes)
    printf '%%2\t1\n'
    ;;
esac
`,
			wantContains: []string{"kill-session -t =alpha"},
		},
		{
			name: "ignores refresh failure after successful kill",
			script: `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TMUX_LOG"
case "$1" in
  list-sessions)
    printf '$1\talpha\t1\t1\n$2\tbeta\t1\t0\n'
    ;;
  kill-session)
    exit 0
    ;;
  *)
    exit 1
    ;;
esac
`,
			wantContains: []string{"kill-session -t =alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			logPath := installFakeTmux(t, tt.script)
			sidebar := mocks.NewMockTmuxSidebarPort(t)
			sidebar.EXPECT().RefreshSidebar(ctx, "/dev/pts/99").Return(errors.New("refresh failure"))
			err := killSession(ctx, map[string]string{"client": "/dev/pts/99", "session": "alpha", "confirmed": "yes"}, sidebar)
			if err != nil {
				t.Fatalf("killSession returned error: %v", err)
			}
			log := readLog(t, logPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(log, want) {
					t.Fatalf("expected log to contain %q, log=%q", want, log)
				}
			}
		})
	}
}

func assertPersistedMetadata(t *testing.T, name string, want ports.SessionMetadata) {
	t.Helper()
	state, err := loadSidebarState(t.Context())
	if err != nil {
		t.Fatalf("loadSidebarState error = %v", err)
	}
	if got, ok := state.Sessions[name]; !ok || got != want {
		t.Fatalf("Sessions[%q] = %#v, %v; want %#v, true", name, got, ok, want)
	}
}

func assertPersistedOrder(t *testing.T, want []string) {
	t.Helper()
	state, err := loadSidebarState(t.Context())
	if err != nil {
		t.Fatalf("loadSidebarState error = %v", err)
	}
	if !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, want)
	}
}

func seedPersistedState(t *testing.T, sessions map[string]ports.SessionMetadata, order []string) {
	t.Helper()
	store := sessionOrderStore()
	state, err := store.Load(t.Context(), "tmux")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state.Sessions = sessions
	state.SessionOrder = order
	if err := store.Save(t.Context(), "tmux", state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	logBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	return string(logBytes)
}
