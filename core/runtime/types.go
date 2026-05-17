package runtime

import (
	"github.com/bnema/tmux-session-sidebar/core/clients"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type State struct {
	Config   ports.ConfigSnapshot
	Sessions map[string]ports.TmuxSessionSnapshot
	Clients  map[string]clients.State
	Sidebars map[string]sidebar.State
	Heat     map[string]heat.State
	Panes    map[string]ports.PaneRef
}

type Service struct {
	tmuxConfig ports.TmuxConfigPort
	tmuxQuery  ports.TmuxQueryPort
	tmuxCtl    ports.TmuxControlPort
	store      ports.StateStorePort
}

func NewService(config ports.TmuxConfigPort, query ports.TmuxQueryPort, control ports.TmuxControlPort, store ports.StateStorePort) *Service {
	return &Service{tmuxConfig: config, tmuxQuery: query, tmuxCtl: control, store: store}
}
