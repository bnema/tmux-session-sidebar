package ports

import (
	"context"
	"errors"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

type ConfigSnapshot struct {
	Loaded                  bool
	KeyBinding              string
	Width                   string
	ProjectRoots            []string
	CloseAfterSwitch        bool
	HeatColorsEnabled       bool
	HeatHalfLifeHours       int
	HeatStaleHours          int
	HeatRefreshSeconds      int
	HeatRecentInterval      time.Duration
	HeatMaxHighlighted      int
	ActivityDebugLog        bool
	AgentAttentionEnabled   bool
	AgentAttentionAnimation config.AgentAttentionAnimation
	AutoSortRecentInterval  time.Duration
	RestoreSessionsMode     string
	ContinuumGraceSeconds   int
	ColorSchemeMode         config.ColorSchemeMode
	ColorSchemeAppearance   config.ColorSchemeAppearance
	MetadataSublineEnabled  bool
	MetadataInactiveEnabled bool
}

type SessionSnapshot struct {
	ID            string
	Name          string
	WindowCount   int
	AttachedCount int
}

type ClientSnapshot struct {
	ID               string
	CurrentSessionID string
	CurrentWindowID  string
	CurrentPaneID    string
	Attached         bool
}

type PaneSnapshot struct {
	PaneID      string
	SessionID   string
	SessionName string
	WindowID    string
	CurrentPath string
	CurrentCmd  string
	Dead        bool
	DeadStatus  string
	Sidebar     bool
}

type PaneSize struct {
	Width  int
	Height int
}

var ErrMultiplexerTargetGone = errors.New("multiplexer target gone")

type PaneRef struct {
	PaneID   string
	WindowID string
}

type SessionMetadata struct {
	Kind        string `json:"kind,omitempty"`
	ProjectPath string `json:"projectPath,omitempty"`
	LastPath    string `json:"lastPath,omitempty"`
}

type ConfigPort interface {
	LoadConfig(ctx context.Context) (ConfigSnapshot, error)
}

type QueryPort interface {
	ServerID(ctx context.Context) (string, error)
	ListSessions(ctx context.Context) ([]SessionSnapshot, error)
	ListClients(ctx context.Context) ([]ClientSnapshot, error)
	CurrentPanePath(ctx context.Context, clientID string) (string, error)
	SessionPath(ctx context.Context, sessionName string) (string, error)
	PaneSize(ctx context.Context, paneID string) (PaneSize, error)
}

type ControlPort interface {
	SwitchClientSession(ctx context.Context, clientID string, sessionName string) error
	DisplayMessage(ctx context.Context, clientID string, message string) error
	CreateSession(ctx context.Context, sessionName string, path string) error
	RenameSession(ctx context.Context, oldName string, newName string) error
	KillSession(ctx context.Context, sessionName string) error
}

type SidebarPort interface {
	CloseAfterSwitch(ctx context.Context) (bool, error)
	FindSidebarPane(ctx context.Context, target string) (PaneRef, error)
	FindSingletonSidebar(ctx context.Context) (PaneRef, error)
	EnsureSingletonSidebar(ctx context.Context, command []string) (PaneRef, error)
	AttachSingletonSidebar(ctx context.Context, clientID string, paneID string, width string) (PaneRef, error)
	ParkSingletonSidebar(ctx context.Context, paneID string) error
	RefreshSidebar(ctx context.Context, clientID string) error
	ScheduleSidebarRestoreOnExit(ctx context.Context, clientID string, paneID string) error
}

type SidebarSwitchPort interface {
	// AttachSingletonSidebarAndSwitchClient moves the sidebar to sessionName,
	// switches the client there, and leaves focus on the work pane next to the
	// sidebar instead of focusing the sidebar pane itself.
	AttachSingletonSidebarAndSwitchClient(ctx context.Context, clientID string, sessionName string, paneID string, width string) error
}

type SidebarFollowPort interface {
	// AttachSingletonSidebarWithoutFocus attaches the sidebar while preserving
	// focus in the work pane. Callers may fall back to AttachSingletonSidebar
	// when an adapter does not implement this optional behavior.
	AttachSingletonSidebarWithoutFocus(ctx context.Context, clientID string, paneID string, width string) (PaneRef, error)
}

type SidebarResizePort interface {
	// CaptureAttachedSidebarWidthBaseline records the current sidebar-open
	// top-level work-group proportions when the attached layout is in a stable
	// shape. Intended for non-resize layout-change hooks and post-attach capture.
	CaptureAttachedSidebarWidthBaseline(ctx context.Context, windowID string, paneID string, width string) error
	// SyncAttachedSidebarWidth restores the configured width for an already
	// attached sidebar pane while best-effort preserving the last captured
	// sidebar-open top-level work-group proportions in the same window.
	// Intended for resize hooks.
	SyncAttachedSidebarWidth(ctx context.Context, windowID string, paneID string, width string) error
}

type MetadataPort interface {
	LoadSessionMetadata(ctx context.Context, sessionName string) (SessionMetadata, error)
	SaveSessionMetadata(ctx context.Context, sessionName string, metadata SessionMetadata) error
}

type CommandPort interface {
	Run(ctx context.Context, args []string) (Result, error)
}

type RuntimePort interface {
	ConfigPort
	QueryPort
	ControlPort
	SidebarPort
	MetadataPort
	SidebarRefresherPort
	CommandPort
}
