package app

import "github.com/bnema/tmux-session-sidebar/adapters/uity"

type sidebarTreeRowKind string

const (
	sidebarTreeRowCategory  sidebarTreeRowKind = "category"
	sidebarTreeRowSession   sidebarTreeRowKind = "session"
	sidebarTreeRowSeparator sidebarTreeRowKind = "separator"
	sidebarTreeRowSpacer    sidebarTreeRowKind = "spacer"
)

type sidebarTreeItem struct {
	Kind           sidebarTreeRowKind
	ID             string
	CategoryID     string
	CategoryName   string
	CategoryOpen   bool
	Session        uity.SessionItem
	Slot           int
	Branch         string
	MetadataPrefix string
	ShowMetadata   bool
}
