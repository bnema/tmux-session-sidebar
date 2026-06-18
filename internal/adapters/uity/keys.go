package uity

type Mode string

const (
	ModeBrowse         Mode = "browse"
	ModeSearch         Mode = "search"
	ModeProject        Mode = "project"
	ModeCreate         Mode = "create"
	ModeCreateNamed    Mode = "create-named"
	ModeCreateCategory Mode = "create-category"
	ModeRenameCategory Mode = "rename-category"
	ModeConfirmKill    Mode = "confirm-kill"
	ModeConfirmDelete  Mode = "confirm-delete"
	ModePinColor       Mode = "pin-color"
)
