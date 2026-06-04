package app

import (
	"time"

	"github.com/bnema/tmux-session-sidebar/ports"
)

var metadataGitStatusTimeout = time.Second

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

func gitStatusEqual(left, right ports.GitStatus) bool {
	return left == right
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
