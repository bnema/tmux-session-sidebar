package uity

type Mode string

const (
	ModeBrowse      Mode = "browse"
	ModeSearch      Mode = "search"
	ModeProject     Mode = "project"
	ModeConfirmKill Mode = "confirm-kill"
	ModePinColor    Mode = "pin-color"
)
