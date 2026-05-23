package runtime

import (
	"github.com/bnema/tmux-session-sidebar/core/clients"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

// State holds the aggregated runtime state for one tmux server. Sessions and
// Heat are keyed by session ID or name depending on source data, while Clients,
// Sidebars, and Panes are keyed by tmux client ID.
type State struct {
	Config   ports.ConfigSnapshot
	Sessions map[string]ports.TmuxSessionSnapshot
	Clients  map[string]clients.State
	Sidebars map[string]sidebar.State
	Heat     map[string]heat.State
	Panes    map[string]ports.PaneRef
}

// NewState returns a State with all map fields initialized.
func NewState() State {
	return State{
		Sessions: map[string]ports.TmuxSessionSnapshot{},
		Clients:  map[string]clients.State{},
		Sidebars: map[string]sidebar.State{},
		Heat:     map[string]heat.State{},
		Panes:    map[string]ports.PaneRef{},
	}
}

// Service coordinates runtime behavior by composing tmux configuration, query,
// control, and persistence ports.
type Service struct {
	tmuxConfig  ports.TmuxConfigPort
	tmuxQuery   ports.TmuxQueryPort
	tmuxCtl     ports.TmuxControlPort
	tmuxSidebar ports.TmuxSidebarPort
	tmuxMeta    ports.TmuxMetadataPort
	store       ports.StateStorePort
	logger      ports.LoggerPort
}

// NewService creates a Service from its adapter ports. Methods validate the
// specific port they use so tests and partial command paths can compose only the
// collaborators needed for that behavior.
func NewService(config ports.TmuxConfigPort, query ports.TmuxQueryPort, control ports.TmuxControlPort, store ports.StateStorePort) *Service {
	return &Service{tmuxConfig: config, tmuxQuery: query, tmuxCtl: control, store: store}
}

func (s *Service) WithSidebar(sidebar ports.TmuxSidebarPort) *Service {
	s.tmuxSidebar = sidebar
	return s
}

func (s *Service) WithMetadata(meta ports.TmuxMetadataPort) *Service {
	s.tmuxMeta = meta
	return s
}

func (s *Service) WithLogger(logger ports.LoggerPort) *Service {
	s.logger = logger
	return s
}
