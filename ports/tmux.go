package ports

import "context"

type ConfigSnapshot struct {
	KeyBinding         string
	Width              string
	ProjectRoots       []string
	CloseAfterSwitch   bool
	HeatColorsEnabled  bool
	HeatHalfLifeHours  int
	HeatStaleHours     int
	HeatRefreshSeconds int
}

type TmuxSessionSnapshot struct {
	ID            string
	Name          string
	WindowCount   int
	AttachedCount int
}

type TmuxClientSnapshot struct {
	ID               string
	CurrentSessionID string
	CurrentWindowID  string
	CurrentPaneID    string
	Attached         bool
}

type PaneSize struct {
	Width  int
	Height int
}

type PaneRef struct {
	PaneID   string
	WindowID string
}

type SessionMetadata struct {
	Kind        string
	ProjectPath string
}

type TmuxConfigPort interface {
	LoadConfig(ctx context.Context) (ConfigSnapshot, error)
}

type TmuxQueryPort interface {
	ServerID(ctx context.Context) (string, error)
	ListSessions(ctx context.Context) ([]TmuxSessionSnapshot, error)
	ListClients(ctx context.Context) ([]TmuxClientSnapshot, error)
	CurrentPanePath(ctx context.Context, clientID string) (string, error)
	PaneSize(ctx context.Context, paneID string) (PaneSize, error)
}

type TmuxControlPort interface {
	SwitchClientSession(ctx context.Context, clientID string, sessionName string) error
	DisplayMessage(ctx context.Context, clientID string, message string) error
	CreateSession(ctx context.Context, sessionName string, path string) error
	RenameSession(ctx context.Context, oldName string, newName string) error
	KillSession(ctx context.Context, sessionName string) error
}

type TmuxSidebarPort interface {
	CloseAfterSwitch(ctx context.Context) (bool, error)
	FindSidebarPane(ctx context.Context, target string) (PaneRef, error)
	OpenSidebar(ctx context.Context, clientID string, command []string) (PaneRef, error)
	CloseSidebar(ctx context.Context, clientID string) error
	CloseSidebarPane(ctx context.Context, paneID string) error
	RefreshSidebar(ctx context.Context, clientID string) error
	ScheduleSidebarRestoreOnExit(ctx context.Context, clientID string, paneID string) error
}

type TmuxMetadataPort interface {
	LoadSessionMetadata(ctx context.Context, sessionName string) (SessionMetadata, error)
	SaveSessionMetadata(ctx context.Context, sessionName string, metadata SessionMetadata) error
}
