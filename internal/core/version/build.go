package version

import (
	"strconv"
	"strings"
)

type BuildKind string

const (
	BuildKindUnknown BuildKind = "unknown"
	BuildKindRelease BuildKind = "release"
	BuildKindSource  BuildKind = "source"
)

type BuildInfoFallback struct {
	Version  string
	Revision string
	Time     string
	Modified string
}

type BuildMetadata struct {
	Kind        BuildKind
	Version     string
	Tag         string
	Commit      string
	Date        string
	BuiltBy     string
	Distance    int
	HasDistance bool
	Dirty       bool
	DirtyKnown  bool
}

func (m BuildMetadata) Display() string {
	if m.Kind == BuildKindRelease {
		if normalized := NormalizeReleaseVersion(m.Version); normalized != "" {
			return normalized
		}
		if normalized := NormalizeReleaseVersion(m.Tag); normalized != "" {
			return normalized
		}
	}

	shortCommit := m.ShortCommit()
	if tag := NormalizeReleaseVersion(m.Tag); tag != "" && shortCommit != "" && m.HasDistance {
		return tag + "+" + m.DistanceString() + ".g" + shortCommit + m.dirtySuffix()
	}
	if shortCommit != "" {
		return "dev.g" + shortCommit + m.dirtySuffix()
	}
	return "dev"
}

func (m BuildMetadata) ReleaseCheckVersion() string {
	if m.Kind != BuildKindRelease {
		return ""
	}
	if normalized := NormalizeReleaseVersion(m.Version); normalized != "" {
		return normalized
	}
	return NormalizeReleaseVersion(m.Tag)
}

func (m BuildMetadata) ShortCommit() string {
	commit := cleanSetting(m.Commit)
	if commit == "" {
		return ""
	}
	if len(commit) < 7 {
		return commit
	}
	return commit[:7]
}

func (m BuildMetadata) IsSource() bool { return m.Kind == BuildKindSource }

func (m BuildMetadata) DistanceString() string {
	if !m.HasDistance {
		return ""
	}
	return strconv.Itoa(m.Distance)
}

func (m BuildMetadata) DirtyString() string {
	if !m.DirtyKnown {
		return "unknown"
	}
	return strconv.FormatBool(m.Dirty)
}

func (m BuildMetadata) DetailCommit() string {
	if commit := cleanSetting(m.Commit); commit != "" {
		return commit
	}
	return "unknown"
}

func (m BuildMetadata) DetailDate() string {
	if date := cleanSetting(m.Date); date != "" {
		return date
	}
	return "unknown"
}

func (m BuildMetadata) dirtySuffix() string {
	if m.DirtyKnown && m.Dirty {
		return "*"
	}
	return ""
}

func NormalizeReleaseVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	candidate := strings.TrimPrefix(raw, "v")
	if strings.HasPrefix(candidate, "v") {
		return ""
	}
	if _, ok := parseSemanticVersion(candidate); !ok {
		return ""
	}
	return "v" + candidate
}

func FromBuildSettings(version, commit, date, builtBy, tag, distance, dirty string, fallback BuildInfoFallback) BuildMetadata {
	version = cleanSetting(version)
	if fallbackVersion := cleanSetting(fallback.Version); fallbackVersion != "" && (version == "" || strings.EqualFold(version, "dev")) {
		version = fallbackVersion
	}
	commit = firstKnown(commit, fallback.Revision)
	date = firstKnown(date, fallback.Time)
	builtBy = cleanSetting(builtBy)
	tag = cleanSetting(tag)

	distanceValue, hasDistance := parseDistance(distance)
	dirtyValue, dirtyKnown := parseDirty(dirty)
	if !dirtyKnown {
		dirtyValue, dirtyKnown = parseDirty(fallback.Modified)
	}

	kind := BuildKindUnknown
	switch {
	case strings.EqualFold(builtBy, "goreleaser"):
		kind = BuildKindRelease
	case tag != "" || hasDistance || strings.EqualFold(version, "dev"):
		kind = BuildKindSource
	case NormalizeReleaseVersion(version) != "":
		kind = BuildKindRelease
	case strings.EqualFold(builtBy, "source") || version == "":
		kind = BuildKindSource
	}

	if kind == BuildKindRelease {
		if tag == "" {
			tag = NormalizeReleaseVersion(version)
		}
		if !hasDistance {
			distanceValue = 0
			hasDistance = true
		}
		if !dirtyKnown {
			dirtyValue = false
			dirtyKnown = true
		}
	}
	if version == "" {
		version = "dev"
	}
	if builtBy == "" {
		builtBy = "source"
	}

	return BuildMetadata{
		Kind:        kind,
		Version:     version,
		Tag:         tag,
		Commit:      commit,
		Date:        date,
		BuiltBy:     builtBy,
		Distance:    distanceValue,
		HasDistance: hasDistance,
		Dirty:       dirtyValue,
		DirtyKnown:  dirtyKnown,
	}
}

func firstKnown(values ...string) string {
	for _, value := range values {
		if cleaned := cleanSetting(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func cleanSetting(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "unknown") || value == "<unknown>" || value == "(devel)" {
		return ""
	}
	return value
}

func parseDistance(raw string) (int, bool) {
	raw = cleanSetting(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func parseDirty(raw string) (bool, bool) {
	switch strings.ToLower(cleanSetting(raw)) {
	case "true", "1", "yes", "dirty":
		return true, true
	case "false", "0", "no", "clean":
		return false, true
	default:
		return false, false
	}
}
