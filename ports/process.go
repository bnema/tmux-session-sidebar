package ports

import "context"

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type ProcessPort interface {
	Exec(ctx context.Context, name string, args []string) (Result, error)
}
