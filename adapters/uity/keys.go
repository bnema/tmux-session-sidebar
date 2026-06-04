package uity

type Mode string

const (
	ModeBrowse         Mode = "browse"
	ModeSearch         Mode = "search"
	ModeProject        Mode = "project"
	ModeNewItem        Mode = "new-item"
	ModeCreateSession  Mode = "create-session"
	ModeCreateNamed    Mode = "create-named"
	ModeRenameCategory Mode = "rename-category"
	ModeConfirmKill    Mode = "confirm-kill"
	ModePinColor       Mode = "pin-color"
)
