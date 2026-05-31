package app

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

var metadataCaptureTimeout = 10 * time.Second
var metadataGitStatusTimeout = 250 * time.Millisecond
var metadataCaptureInFlight atomic.Bool

func captureSessionMetadataAsync(ctx context.Context, cfg ports.ConfigSnapshot) {
	if ctx.Value(disableAsyncMetadataCaptureKey{}) != nil || !cfg.MetadataSublineEnabled || !metadataCaptureInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer metadataCaptureInFlight.Store(false)
		captureCtx, cancel := context.WithTimeout(ctx, metadataCaptureTimeout)
		defer cancel()
		if err := NewMetadataService().CaptureAndRefresh(captureCtx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "tmux-session-sidebar: metadata capture failed: %v\n", err)
		}
	}()
}

type disableAsyncMetadataCaptureKey struct{}

func sessionMetadataCapturePath(sessionName string, metadata ports.SessionMetadata, livePaths map[string]string) (string, bool) {
	if path := livePaths[sessionName]; path != "" {
		return path, true
	}
	return sessionMetadataPath(metadata)
}

func sessionMetadataPath(metadata ports.SessionMetadata) (string, bool) {
	if metadata.ProjectPath != "" {
		return metadata.ProjectPath, true
	}
	if metadata.LastPath != "" {
		return metadata.LastPath, true
	}
	return "", false
}

func gitMetadataEqual(left, right map[string]ports.GitStatus) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok || leftValue != rightValue {
			return false
		}
	}
	return true
}
