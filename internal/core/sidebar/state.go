package sidebar

type Mode string

const (
	ModeBrowse Mode = "browse"
	ModeSearch Mode = "search"
)

type State struct {
	Open                bool
	ShowNumericSessions bool
	Filter              string
	SelectionSessionID  string
	Mode                Mode
}
