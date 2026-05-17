package app

// Route is the parsed command requested by the tmux bootstrap, hooks, UI pane,
// or direct user invocation.
type Route struct {
	Path  string
	Flags map[string]string
	Args  []string
}
