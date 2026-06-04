package uity

type Mode string

const (
	ModeBrowse         Mode = "browse"
	ModeSearch         Mode = "search"
	ModeProject        Mode = "project"
	ModeCreate         Mode = "create"
	ModeCreateNamed    Mode = "create-named"
	ModeRenameCategory Mode = "rename-category"
	ModeConfirmKill    Mode = "confirm-kill"
	ModePinColor       Mode = "pin-color"
)
