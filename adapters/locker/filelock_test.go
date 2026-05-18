package locker

import "testing"

func TestNilHandleReleaseIsNoop(t *testing.T) {
	var handle *Handle
	if err := handle.Release(); err != nil {
		t.Fatalf("Release error = %v, want nil", err)
	}
}
