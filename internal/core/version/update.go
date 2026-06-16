package version

import (
	"strconv"
	"strings"
)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func UpdateAvailable(current string, latest string) bool {
	current = strings.TrimSpace(current)
	if current == "dev" {
		return true
	}
	currentVersion, ok := parseSemanticVersion(current)
	if !ok {
		return false
	}
	latestVersion, ok := parseSemanticVersion(latest)
	if !ok {
		return false
	}
	return compareSemanticVersion(latestVersion, currentVersion) > 0
}

func CheckableReleaseVersion(version string) bool {
	_, ok := parseSemanticVersion(version)
	return ok
}

func parseSemanticVersion(raw string) (semanticVersion, bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	major, ok := parseVersionPart(parts[0])
	if !ok {
		return semanticVersion{}, false
	}
	minor, ok := parseVersionPart(parts[1])
	if !ok {
		return semanticVersion{}, false
	}
	patch, ok := parseVersionPart(parts[2])
	if !ok {
		return semanticVersion{}, false
	}
	return semanticVersion{major: major, minor: minor, patch: patch}, true
}

func parseVersionPart(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func compareSemanticVersion(left semanticVersion, right semanticVersion) int {
	if left.major != right.major {
		return left.major - right.major
	}
	if left.minor != right.minor {
		return left.minor - right.minor
	}
	return left.patch - right.patch
}
