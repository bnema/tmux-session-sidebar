package ports

type FilesystemPort interface {
	ResolvePath(path string) (string, error)
	ListImmediateDirs(root string) ([]string, error)
}
