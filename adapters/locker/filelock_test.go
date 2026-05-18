package locker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAcquireRespectsContextWhenLockIsHeld(t *testing.T) {
	locker := FileLocker{Dir: t.TempDir()}
	first, err := locker.Acquire(context.Background(), "state")
	if err != nil {
		t.Fatalf("first Acquire error = %v", err)
	}
	defer func() { _ = first.Release() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	second, err := locker.Acquire(ctx, "state")
	if second != nil {
		_ = second.Release()
		t.Fatal("second Acquire returned handle while lock was held")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Acquire error = %v, want context deadline exceeded", err)
	}
}

func TestNilHandleReleaseIsNoop(t *testing.T) {
	var handle *Handle
	if err := handle.Release(); err != nil {
		t.Fatalf("Release error = %v, want nil", err)
	}
}
