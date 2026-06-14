package viewmodel

// SessionItem describes one session row as presented to a UI adapter.
// Producers are expected to populate HeatIntensity and InactiveIntensity in the
// 0.0-1.0 range. UI consumers clamp out-of-range values defensively, but such
// values should be treated as a producer bug rather than a supported input.
type SessionItem struct {
	// Name is the tmux session name.
	Name string
	// Current reports whether this is the active session.
	Current bool
	// Slot is the visible quick-switch slot assigned to the session. It remains
	// on SessionItem so detached session lists can carry the slot without a
	// wrapping TreeItem.
	Slot int
	// Heat is the symbolic heat bucket chosen for the session.
	Heat string
	// HeatIntensity is the normalized highlight strength for Heat. Producers
	// should provide a value in the 0.0-1.0 range; consumers clamp defensively.
	HeatIntensity float64
	// InactiveIntensity is the normalized recency highlight for inactive rows.
	// Producers should provide a value in the 0.0-1.0 range; consumers clamp
	// defensively.
	InactiveIntensity float64
	// Attention reports whether the session has unread agent attention.
	Attention bool
	// Pinned reports whether the session is pinned.
	Pinned bool
	// PinColor is the configured color tag for a pinned session.
	PinColor string
	// Metadata is the optional secondary line shown under the session.
	Metadata SessionMetadataSubline
}

// ProjectItem represents a project entry with display name and filesystem path.
type ProjectItem struct {
	// Name is the display label for the project.
	Name string
	// Path is the project root path.
	Path string
}

// TreeRowKind identifies the kind of row represented in the sidebar tree.
type TreeRowKind string

const (
	// TreeRowCategory is a category header row.
	TreeRowCategory TreeRowKind = "category"
	// TreeRowSession is a session entry row.
	TreeRowSession TreeRowKind = "session"
	// TreeRowSeparator is a visual separator row.
	TreeRowSeparator TreeRowKind = "separator"
	// TreeRowSpacer is an empty spacer row.
	TreeRowSpacer TreeRowKind = "spacer"
	// TreeRowMore is an overflow-expansion row.
	TreeRowMore TreeRowKind = "more"
)

// TreeItem represents a single sidebar tree row.
//
// Field usage depends on Kind:
//   - TreeRowCategory uses CategoryID, CategoryName, CategoryOpen, and Color.
//   - TreeRowSession uses Session, Slot, ShowMetadata, CategoryID, and may set
//     OverflowHidden when the row is hidden behind an overflow expander.
//   - TreeRowMore uses MoreCount, MoreExpanded, and may set OverflowHidden when
//     the expander itself is hidden.
//   - TreeRowSeparator and TreeRowSpacer primarily use ID, Depth, and
//     LastChild.
type TreeItem struct {
	// Kind identifies which row variant this item represents.
	Kind TreeRowKind
	// ID is the stable row identifier.
	ID string
	// CategoryID is the owning category identifier when applicable.
	CategoryID string
	// CategoryName is the rendered category label for category rows.
	CategoryName string
	// CategoryOpen reports whether the category is expanded.
	CategoryOpen bool
	// Color is the configured category/session accent color.
	Color string
	// Session holds the row payload for session rows.
	Session SessionItem
	// Slot is the visible quick-switch slot for session rows. It mirrors
	// Session.Slot so tree-level navigation and rendering can read the slot
	// directly from the row model.
	Slot int
	// Depth is the tree indentation depth.
	Depth int
	// LastChild reports whether the row is the last child in its group.
	LastChild bool
	// ShowMetadata controls whether the session metadata subline is shown.
	ShowMetadata bool
	// MoreCount is the number of hidden sessions represented by a more-row.
	MoreCount int
	// MoreExpanded reports whether an overflow row is expanded.
	MoreExpanded bool
	// OverflowHidden reports whether the row is currently hidden by overflow.
	OverflowHidden bool
}
