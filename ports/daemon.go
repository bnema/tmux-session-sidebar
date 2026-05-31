package ports

import "context"

// DaemonLauncherPort starts or verifies the background control-plane daemon.
// Implementations should return quickly; expensive supervision belongs outside
// the latency-sensitive sidebar route path.
type DaemonLauncherPort interface {
	EnsureStarted(ctx context.Context) error
}
