package app

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestDependenciesUseGeneratedPortMocks(t *testing.T) {
	deps := Dependencies{
		TmuxConfig:  &mocks.MockTmuxConfigPort{},
		TmuxQuery:   &mocks.MockTmuxQueryPort{},
		TmuxControl: &mocks.MockTmuxControlPort{},
		TmuxSidebar: &mocks.MockTmuxSidebarPort{},
		TmuxMeta:    &mocks.MockTmuxMetadataPort{},
		IPCClient:   &mocks.MockIPCClientPort{},
		IPCServer:   &mocks.MockIPCServerPort{},
		Store:       &mocks.MockStateStorePort{},
		Clock:       &mocks.MockClockPort{},
		Git:         &mocks.MockGitPort{},
		Filesystem:  &mocks.MockFilesystemPort{},
		Locker:      &mocks.MockLockerPort{},
		Process:     &mocks.MockProcessPort{},
		Logger:      &mocks.MockLoggerPort{},
	}

	tests := []struct {
		name string
		ok   bool
	}{
		{name: "tmux config mock wired", ok: deps.TmuxConfig != nil},
		{name: "tmux query mock wired", ok: deps.TmuxQuery != nil},
		{name: "tmux control mock wired", ok: deps.TmuxControl != nil},
		{name: "tmux sidebar mock wired", ok: deps.TmuxSidebar != nil},
		{name: "tmux metadata mock wired", ok: deps.TmuxMeta != nil},
		{name: "ipc client mock wired", ok: deps.IPCClient != nil},
		{name: "ipc server mock wired", ok: deps.IPCServer != nil},
		{name: "store mock wired", ok: deps.Store != nil},
		{name: "clock mock wired", ok: deps.Clock != nil},
		{name: "git mock wired", ok: deps.Git != nil},
		{name: "filesystem mock wired", ok: deps.Filesystem != nil},
		{name: "locker mock wired", ok: deps.Locker != nil},
		{name: "process mock wired", ok: deps.Process != nil},
		{name: "logger mock wired", ok: deps.Logger != nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok {
				t.Fatal("expected dependency slot to accept generated mock")
			}
		})
	}
}
