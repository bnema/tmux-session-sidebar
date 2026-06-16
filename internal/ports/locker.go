package ports

import "context"

type LockHandle interface {
	Release() error
}

type LockerPort interface {
	Acquire(ctx context.Context, key string) (LockHandle, error)
}
