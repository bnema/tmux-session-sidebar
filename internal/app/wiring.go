package app

import "github.com/bnema/tmux-session-sidebar/ports"

type Dependencies struct {
	TmuxConfig  ports.TmuxConfigPort
	TmuxQuery   ports.TmuxQueryPort
	TmuxControl ports.TmuxControlPort
	TmuxMeta    ports.TmuxMetadataPort
	IPCClient   ports.IPCClientPort
	IPCServer   ports.IPCServerPort
	Store       ports.StateStorePort
	Clock       ports.ClockPort
	Git         ports.GitPort
	Filesystem  ports.FilesystemPort
	Locker      ports.LockerPort
	Process     ports.ProcessPort
	Logger      ports.LoggerPort
}
