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
	MetadataNerdGit        = ""      // git branch glyph
	MetadataNerdGitCompare = "\uf440" // nf-oct-diff; desired compare glyph in supported Nerd Fonts
	MetadataNerdStaged     = "\uf45e" // nf-oct-checklist; staged/index changes
	MetadataNerdWorktree   = "\uf448" // nf-oct-pencil; unstaged/untracked worktree changes
	MetadataNerdWarning    = "\uf071" // nf-fa-warning
	MetadataNerdDirectory  = "\uf115" // nf-fa-folder_open_o
	MetadataNerdShell      = "\uf120" // nf-fa-terminal
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
	Kind            MetadataKind
	SessionName     string
	Branch          string
	Clean           bool
	Ahead           int
	Behind          int
	Staged          int
	Modified        int
	Deleted         int
	Renamed         int
	Untracked       int
	Conflicts       int
	UpstreamMissing bool
	Path            string
	Label           string
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
	return metadataPartText(formatGitMetadataSublineParts(meta, icons, width))
}

type metadataPartRole string

const (
	metadataPartBase     metadataPartRole = "base"
	metadataPartCompare  metadataPartRole = "compare"
	metadataPartAhead    metadataPartRole = "ahead"
	metadataPartBehind   metadataPartRole = "behind"
	metadataPartStaged   metadataPartRole = "staged"
	metadataPartUnstaged metadataPartRole = "unstaged"
	metadataPartConflict metadataPartRole = "conflict"
)

type metadataPart struct {
	Text string
	Role metadataPartRole
}

type gitDetailLevel int

const (
	gitDetailsFull gitDetailLevel = iota
	gitDetailsSummary
	gitDetailsDivergence
)

func formatGitMetadataSublineParts(meta SessionMetadataSubline, icons MetadataIconMode, width int) []metadataPart {
	partSets := [][]metadataPart{
		gitDetailParts(meta, icons, gitDetailsFull),
		gitDetailParts(meta, icons, gitDetailsSummary),
		gitDetailParts(meta, icons, gitDetailsDivergence),
	}
	for _, details := range partSets {
		parts := withBranchPart(meta, icons, width, details)
		line := metadataPartText(parts)
		if line != "" && metadataDisplayWidth(line) <= width {
			return parts
		}
	}
	branch := gitBranchPart(meta, icons, width)
	if branch.Text != "" {
		return []metadataPart{branch}
	}
	return nil
}

func withBranchPart(meta SessionMetadataSubline, icons MetadataIconMode, width int, details []metadataPart) []metadataPart {
	branchBudget := width
	if detailText := metadataPartText(details); detailText != "" {
		branchBudget = width - metadataDisplayWidth(detailText) - 1
	}
	branch := gitBranchPart(meta, icons, branchBudget)
	if branch.Text == "" {
		return details
	}
	parts := make([]metadataPart, 0, len(details)+1)
	parts = append(parts, branch)
	parts = append(parts, details...)
	return parts
}

func gitBranchPart(meta SessionMetadataSubline, icons MetadataIconMode, width int) metadataPart {
	branch := strings.TrimSpace(meta.Branch)
	if branch == "" {
		return metadataPart{}
	}
	prefix := "git "
	if icons == MetadataIconsNerd {
		prefix = MetadataNerdGit + " "
	}
	if width < metadataDisplayWidth(prefix)+2 {
		return metadataPart{}
	}
	return metadataPart{Text: fitMetadataText(prefix+branch, width, icons), Role: metadataPartBase}
}

func gitDetailParts(meta SessionMetadataSubline, icons MetadataIconMode, level gitDetailLevel) []metadataPart {
	if meta.Clean {
		return []metadataPart{{Text: "clean", Role: metadataPartBase}}
	}
	if !meta.hasDivergence() && meta.stagedCount() == 0 && meta.unstagedCount() == 0 && meta.Conflicts == 0 {
		return nil
	}
	parts := make([]metadataPart, 0, 8)
	if meta.Conflicts > 0 {
		parts = append(parts, countPart(icons, MetadataNerdWarning, "!", meta.Conflicts, metadataPartConflict))
	}
	if meta.Ahead > 0 || meta.Behind > 0 {
		parts = append(parts, divergenceParts(icons, meta.Ahead, meta.Behind)...)
	}
	if level == gitDetailsDivergence {
		return parts
	}
	if level == gitDetailsSummary {
		if unstaged := meta.unstagedCount(); unstaged > 0 {
			parts = append(parts, countPart(icons, MetadataNerdWorktree, "U", unstaged, metadataPartUnstaged))
		} else if staged := meta.stagedCount(); staged > 0 {
			parts = append(parts, countPart(icons, MetadataNerdStaged, "S", staged, metadataPartStaged))
		}
		return parts
	}
	if staged := meta.stagedCount(); staged > 0 {
		parts = append(parts, countPart(icons, MetadataNerdStaged, "S", staged, metadataPartStaged))
	}
	if unstaged := meta.unstagedCount(); unstaged > 0 {
		parts = append(parts, countPart(icons, MetadataNerdWorktree, "U", unstaged, metadataPartUnstaged))
	}
	return parts
}

func divergenceParts(icons MetadataIconMode, ahead int, behind int) []metadataPart {
	parts := make([]metadataPart, 0, 3)
	if icons == MetadataIconsNerd {
		parts = append(parts, metadataPart{Text: MetadataNerdGitCompare, Role: metadataPartCompare})
	}
	if ahead > 0 {
		parts = append(parts, metadataPart{Text: strconv.Itoa(ahead), Role: metadataPartAhead})
	}
	if behind > 0 {
		parts = append(parts, metadataPart{Text: "-" + strconv.Itoa(behind), Role: metadataPartBehind})
	}
	return parts
}

func countPart(icons MetadataIconMode, nerdIcon string, asciiPrefix string, count int, role metadataPartRole) metadataPart {
	if icons == MetadataIconsNerd {
		return metadataPart{Text: nerdIcon + " " + strconv.Itoa(count), Role: role}
	}
	return metadataPart{Text: asciiPrefix + strconv.Itoa(count), Role: role}
}

func metadataPartText(parts []metadataPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, " ")
}

func (m SessionMetadataSubline) hasDivergence() bool {
	return m.Ahead > 0 || m.Behind > 0
}

func (m SessionMetadataSubline) stagedCount() int {
	return m.Staged
}

func (m SessionMetadataSubline) unstagedCount() int {
	return m.Modified + m.Deleted + m.Renamed + m.Untracked
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
