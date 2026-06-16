package runtime

import (
	"github.com/bnema/tmux-session-sidebar/core/clients"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

// State holds the aggregated runtime state for one multiplexer server. Sessions
// and Heat are keyed by session ID or name depending on source data, while
// Clients, Sidebars, and Panes are keyed by multiplexer client ID.
type State struct {
	Config   ports.ConfigSnapshot
	Sessions map[string]ports.SessionSnapshot
	Clients  map[string]clients.State
	Sidebars map[string]sidebar.State
	Heat     map[string]heat.State
	Panes    map[string]ports.PaneRef
}

// NewState returns a State with all map fields initialized.
func NewState() State {
	return State{
		Sessions: map[string]ports.SessionSnapshot{},
		Clients:  map[string]clients.State{},
		Sidebars: map[string]sidebar.State{},
		Heat:     map[string]heat.State{},
		Panes:    map[string]ports.PaneRef{},
	}
}

// Service coordinates runtime behavior by composing multiplexer configuration,
// query, control, and persistence ports.
type Service struct {
	config  ports.ConfigPort
	query   ports.QueryPort
	control ports.ControlPort
	sidebar ports.SidebarPort
	meta    ports.MetadataPort
	store   ports.StateStorePort
	logger  ports.LoggerPort
}

// NewService creates a Service from its adapter ports. Methods validate the
// specific port they use so tests and partial command paths can compose only the
// collaborators needed for that behavior.
func NewService(config ports.ConfigPort, query ports.QueryPort, control ports.ControlPort, store ports.StateStorePort) *Service {
	return &Service{config: config, query: query, control: control, store: store}
}

func (s *Service) WithSidebar(sidebar ports.SidebarPort) *Service {
	s.sidebar = sidebar
	return s
}

func (s *Service) WithMetadata(meta ports.MetadataPort) *Service {
	s.meta = meta
	return s
}

func (s *Service) WithLogger(logger ports.LoggerPort) *Service {
	s.logger = logger
	return s
}
