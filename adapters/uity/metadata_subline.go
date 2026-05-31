package uity

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	lipgloss "charm.land/lipgloss/v2"
)

const (
	MetadataNerdPush      = "\uf403" // nf-oct-repo_push
	MetadataNerdPull      = "\uf404" // nf-oct-repo_pull
	MetadataNerdAdded     = "\uf457" // nf-oct-diff_added
	MetadataNerdModified  = "\uf459" // nf-oct-diff_modified
	MetadataNerdRemoved   = "\uf458" // nf-oct-diff_removed
	MetadataNerdRenamed   = "\uf45a" // nf-oct-diff_renamed
	MetadataNerdQuestion  = "\uf128" // nf-fa-question
	MetadataNerdWarning   = "\uf071" // nf-fa-warning
	MetadataNerdDirectory = "\uf115" // nf-fa-folder_open_o
	MetadataNerdShell     = "\uf120" // nf-fa-terminal
)

type MetadataKind string

const (
	MetadataKindGit       MetadataKind = "git"
	MetadataKindDirectory MetadataKind = "directory"
	MetadataKindAdhoc     MetadataKind = "adhoc"
)

type MetadataIconMode string

const (
	MetadataIconsNerd  MetadataIconMode = "nerd"
	MetadataIconsASCII MetadataIconMode = "ascii"
)

type SessionMetadataSubline struct {
	Kind        MetadataKind
	SessionName string
	Branch      string
	Clean       bool
	Ahead       int
	Behind      int
	Staged      int
	Modified    int
	Deleted     int
	Renamed     int
	Untracked   int
	Conflicts       int
	UpstreamMissing bool
	Path            string
	Label       string
}

type MetadataSublineOptions struct {
	Icons MetadataIconMode
	Width int
}

func FormatMetadataSubline(meta SessionMetadataSubline, options MetadataSublineOptions) string {
	width := options.Width
	if width <= 0 {
		width = 80
	}
	if options.Icons == "" {
		options.Icons = MetadataIconsNerd
	}
	switch meta.Kind {
	case MetadataKindGit:
		return formatGitMetadataSubline(meta, options.Icons, width)
	case MetadataKindDirectory:
		return formatDirectoryMetadataSubline(meta, options.Icons, width)
	case MetadataKindAdhoc:
		return formatAdhocMetadataSubline(meta, options.Icons, width)
	default:
		return fitMetadataText(strings.TrimSpace(meta.Label), width, options.Icons)
	}
}

func formatGitMetadataSubline(meta SessionMetadataSubline, icons MetadataIconMode, width int) string {
	partSets := [][]string{
		gitDetailParts(meta, icons, gitDetailsFull),
		gitDetailParts(meta, icons, gitDetailsSummary),
		gitDetailParts(meta, icons, gitDetailsDivergence),
	}
	for _, parts := range partSets {
		line := strings.Join(parts, " ")
		if line != "" && metadataDisplayWidth(line) <= width {
			return line
		}
	}
	return ""
}

type gitDetailLevel int

const (
	gitDetailsFull gitDetailLevel = iota
	gitDetailsDivergence
	gitDetailsSummary
)

func gitDetailParts(meta SessionMetadataSubline, icons MetadataIconMode, level gitDetailLevel) []string {
	if meta.Clean || (!meta.hasDivergence() && meta.dirtyCount() == 0) {
		return nil
	}
	parts := make([]string, 0, 8)
	if meta.Conflicts > 0 {
		parts = append(parts, countPart(icons, MetadataNerdWarning, "!", meta.Conflicts))
	}
	if meta.Ahead > 0 {
		parts = append(parts, countPart(icons, MetadataNerdPush, "^", meta.Ahead))
	}
	if meta.Behind > 0 {
		parts = append(parts, countPart(icons, MetadataNerdPull, "v", meta.Behind))
	}
	if level == gitDetailsDivergence {
		return parts
	}
	if level == gitDetailsSummary {
		parts = nil
		if dirty := meta.dirtyCount(); dirty > 0 {
			if icons == MetadataIconsNerd {
				parts = append(parts, countPart(icons, MetadataNerdModified, "+", dirty))
			} else {
				parts = append(parts, "dirty"+strconv.Itoa(dirty))
			}
		}
		return parts
	}
	if meta.Staged > 0 {
		parts = append(parts, countPart(icons, MetadataNerdAdded, "+", meta.Staged))
	}
	if meta.Modified > 0 {
		parts = append(parts, countPart(icons, MetadataNerdModified, "~", meta.Modified))
	}
	if meta.Deleted > 0 {
		parts = append(parts, countPart(icons, MetadataNerdRemoved, "-", meta.Deleted))
	}
	if meta.Renamed > 0 {
		parts = append(parts, countPart(icons, MetadataNerdRenamed, "r", meta.Renamed))
	}
	if meta.Untracked > 0 {
		parts = append(parts, countPart(icons, MetadataNerdQuestion, "?", meta.Untracked))
	}
	return parts
}

func countPart(icons MetadataIconMode, nerdIcon string, asciiPrefix string, count int) string {
	if icons == MetadataIconsNerd {
		return nerdIcon + " " + strconv.Itoa(count)
	}
	return asciiPrefix + strconv.Itoa(count)
}

func (m SessionMetadataSubline) hasDivergence() bool {
	return m.Ahead > 0 || m.Behind > 0
}

func (m SessionMetadataSubline) dirtyCount() int {
	return m.Staged + m.Modified + m.Deleted + m.Renamed + m.Untracked + m.Conflicts
}

func formatDirectoryMetadataSubline(meta SessionMetadataSubline, icons MetadataIconMode, width int) string {
	path := compactDisplayPath(meta.Path)
	if path == "" || sameMetadataLabel(path, meta.SessionName) {
		return ""
	}
	prefix := "dir "
	if icons == MetadataIconsNerd {
		prefix = MetadataNerdDirectory + " "
	}
	return fitMetadataText(prefix+path, width, icons)
}

func formatAdhocMetadataSubline(meta SessionMetadataSubline, icons MetadataIconMode, width int) string {
	label := strings.TrimSpace(meta.Label)
	if label == "" {
		label = "adhoc"
	}
	if sameMetadataLabel(label, meta.SessionName) {
		return ""
	}
	prefix := "sh "
	if icons == MetadataIconsNerd {
		prefix = MetadataNerdShell + " "
	}
	return fitMetadataText(prefix+label, width, icons)
}

func compactDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		home = filepath.Clean(home)
		if rel, err := filepath.Rel(home, clean); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	base := filepath.Base(clean)
	parent := filepath.Base(filepath.Dir(clean))
	if strings.HasPrefix(clean, "/") && parent != "." && parent != string(filepath.Separator) {
		return parent + "/" + base
	}
	return base
}

func sameMetadataLabel(value, sessionName string) bool {
	value = strings.Trim(filepath.Base(value), " .")
	sessionName = strings.Trim(sessionName, " .")
	return value != "" && sessionName != "" && value == sessionName
}

func fitMetadataText(value string, width int, icons MetadataIconMode) string {
	value = strings.TrimSpace(value)
	if width <= 0 || metadataDisplayWidth(value) <= width {
		return value
	}
	ellipsis := "…"
	if icons == MetadataIconsASCII {
		ellipsis = "..."
	}
	return trimDisplayRight(value, max(width-metadataDisplayWidth(ellipsis), 0)) + ellipsis
}

func trimDisplayRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range value {
		w := runeDisplayWidth(r)
		if used+w > width {
			break
		}
		used += w
		b.WriteRune(r)
	}
	return b.String()
}

func metadataDisplayWidth(value string) int {
	return lipgloss.Width(sanitizeSessionName(value))
}

func runeDisplayWidth(r rune) int {
	if r == utf8.RuneError || unicode.IsControl(r) {
		return 0
	}
	return lipgloss.Width(string(r))
}

