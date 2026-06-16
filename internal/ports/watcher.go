package ports

import "context"

type FileWatchEvent struct {
	Path string
	Op   string
}

type FileWatcherPort interface {
	Watch(ctx context.Context, paths []string) (<-chan FileWatchEvent, <-chan error, error)
}

type SidebarRefresherPort interface {
	RefreshAllSidebars(ctx context.Context) error
}
