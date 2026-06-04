package uity

import (
	"os"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/bnema/tmux-session-sidebar/core/heat"
)

func padSidebarContentLines(lines []string) []string {
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = " " + line + " "
	}
	return padded
}

func (m SidebarModel) statusBarLines(styles sidebarStyles) []string {
	return []string{m.collapsedHelpLine(styles)}
}

func (m SidebarModel) helpSheetContent(styles sidebarStyles) string {
	lines := []string{
		styles.accent.Render("navigation"),
		"↵ switch    / filter    esc close",
		"j/k move    " + spaceKeySymbol + " pin       h nums",
		"",
		styles.accent.Render("sessions"),
		"c create    r rename    x kill",
		"u update    n layout    J/K",
	}
	return strings.Join(lines, "\n")
}

func (m SidebarModel) collapsedHelpLine(styles sidebarStyles) string {
	version := displayVersion(m.version)
	if version == "" {
		return styles.dim.Render("? keys")
	}
	if m.updateCheck.available {
		return styles.versionBadge.Render(" "+version) + styles.updateIndicator.Render(updateAvailableSymbol+" ") + styles.dim.Render(" ? keys")
	}
	return styles.versionBadge.Render(" "+version+" ") + styles.dim.Render(" ? keys")
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func newSidebarStyles() sidebarStyles {
	return sidebarStyles{
		accent:          lipgloss.NewStyle().Foreground(lipgloss.Color("#7dd3fc")),
		dim:             lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")),
		active:          lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true),
		stale:           lipgloss.NewStyle().Foreground(lipgloss.Color(inactiveSessionRGB.Hex())),
		selected:        lipgloss.NewStyle().Background(lipgloss.Color(selectedRowBackgroundRGB.Hex())).Foreground(lipgloss.Color("#ecfdf5")).Bold(true),
		pinned:          lipgloss.NewStyle().Foreground(lipgloss.Color(defaultPinColor)).Bold(true),
		versionBadge:    lipgloss.NewStyle().Background(lipgloss.Color("#334155")).Foreground(lipgloss.Color("#e0f2fe")).Bold(true),
		updateIndicator: lipgloss.NewStyle().Background(lipgloss.Color("#334155")).Foreground(lipgloss.Color("#22c55e")).Bold(true),
	}
}

func sessionRowStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	if item.Pinned {
		return styles.pinned.Foreground(lipgloss.Color(pinColor(item)))
	}
	if item.Current {
		return styles.active
	}
	if item.Heat == "" || item.Heat == string(heat.BucketStale) {
		return styles.stale
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(heatColor(item.HeatIntensity)))
}

func pinColor(item SessionItem) string {
	if strings.TrimSpace(item.PinColor) == "" {
		return defaultPinColor
	}
	return item.PinColor
}

func (m SidebarModel) statusLine() string {
	switch m.mode {
	case ModeSearch:
		return "filter: " + m.filter
	case ModeProject, ModeNewItem, ModeCreateSession:
		if m.menu.Spec.Filterable && m.menu.Filter != "" {
			return m.menu.Spec.Title + ": " + m.menu.Filter
		}
		return m.menu.Spec.Title
	case ModeCreateNamed:
		return "named session: " + m.createNamedInput
	case ModeRenameCategory:
		return "rename category: " + m.renameCategoryInput
	case ModeConfirmKill:
		return "confirm kill"
	default:
		return ""
	}
}

func metadataColorActive(item SessionItem) bool {
	return item.Current || (item.Heat != "" && item.Heat != string(heat.BucketStale))
}

func bestEffortMetadataIconMode() MetadataIconMode {
	term := strings.ToLower(os.Getenv("TERM"))
	localeValues := []string{}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			localeValues = append(localeValues, value)
		}
	}
	locale := strings.ToLower(strings.Join(localeValues, ":"))
	if term == "dumb" || strings.Contains(locale, "ascii") || (!strings.Contains(locale, "utf") && locale != "") {
		return MetadataIconsASCII
	}
	return MetadataIconsNerd
}
