package app

import (
	"testing"
	"time"
)

// Helper to check if a timer channel has been drained.
func isTimerChannelEmpty(t *time.Timer) bool {
	if t == nil {
		return true
	}
	select {
	case <-t.C:
		return false
	default:
		return true
	}
}

func TestStopTimerBestEffort_Nil(t *testing.T) {
	// Must not panic.
	stopTimerBestEffort(nil)
}

func TestStopTimerBestEffort_StopsLiveTimer(t *testing.T) {
	tmr := time.NewTimer(time.Hour)
	stopTimerBestEffort(tmr)
	if !isTimerChannelEmpty(tmr) {
		t.Fatal("live timer channel should be empty after stop")
	}
}

func TestStopTimerBestEffort_DrainsExpiredTimer(t *testing.T) {
	tmr := time.NewTimer(0)
	// Let the timer fire.
	<-tmr.C

	// The timer channel now has a value; Stop returns false.
	stopTimerBestEffort(tmr)
	if !isTimerChannelEmpty(tmr) {
		t.Fatal("expired timer channel should be drained after stopTimerBestEffort")
	}
}

func TestStopTimerBestEffort_ExpiredTimerDoesNotBlock(t *testing.T) {
	tmr := time.NewTimer(0)
	<-tmr.C // consume the initial fire

	done := make(chan struct{}, 1)
	go func() {
		stopTimerBestEffort(tmr)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// OK — returned without blocking.
	case <-time.After(5 * time.Second):
		t.Fatal("stopTimerBestEffort on an expired timer blocked")
	}
}
