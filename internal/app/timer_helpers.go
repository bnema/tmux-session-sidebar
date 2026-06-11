package app

import "time"

// stopTimerBestEffort stops a timer and drains its channel without blocking.
// It is safe to call with a nil timer. The drain uses a non-blocking select
// instead of an unconditional receive so the call never hangs.
func stopTimerBestEffort(t *time.Timer) {
	if t == nil {
		return
	}
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
