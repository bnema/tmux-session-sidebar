package clients

type State struct {
	ID               string
	CurrentSessionID string
	CurrentWindowID  string
	CurrentPaneID    string
	Attached         bool
}
