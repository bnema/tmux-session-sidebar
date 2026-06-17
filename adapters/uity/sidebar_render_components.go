package uity

import (
	"os"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/bnema/tmux-session-sidebar/internal/core/config"
	"github.com/bnema/tmux-session-sidebar/internal/core/heat"
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
		"j/k move    alt+h nums",
		spaceKeySymbol + " pin      C color",
		"",
		styles.accent.Render("sessions"),
		"c create    r rename    d del",
		"u update    n new      J/K",
	}
	return strings.Join(lines, "\n")
}

func (m SidebarModel) collapsedHelpLine(styles sidebarStyles) string {
	version := displayVersion(m.version)
	if version == "" {
		return styles.dim.Render("? keys")
	}
	versionBadge := styles.versionBadge
	updateIndicator := styles.updateIndicator
	if m.focused {
		focusedBackground := lipgloss.Color(styles.scheme.selectedRowBackgroundRGB.Hex())
		versionBadge = versionBadge.Background(focusedBackground)
		updateIndicator = updateIndicator.Background(focusedBackground)
	}
	if m.updateCheck.available {
		return versionBadge.Render(" "+version) + updateIndicator.Render(updateAvailableSymbol+" ") + styles.dim.Render(" ? keys")
	}
	return versionBadge.Render(" "+version+" ") + styles.dim.Render(" ? keys")
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

type sidebarStyles struct {
	appearance      config.ColorSchemeAppearance
	scheme          sidebarColorScheme
	accent          lipgloss.Style
	dim             lipgloss.Style
	treeGuide       lipgloss.Style
	active          lipgloss.Style
	selected        lipgloss.Style
	pinned          lipgloss.Style
	warning         lipgloss.Style
	destructive     lipgloss.Style
	versionBadge    lipgloss.Style
	updateIndicator lipgloss.Style
}

func newSidebarStyles() sidebarStyles {
	return newSidebarStylesForAppearance(config.ColorSchemeAppearanceDark)
}

func newSidebarStylesForAppearance(appearance config.ColorSchemeAppearance) sidebarStyles {
	scheme := colorScheme(appearance)
	return sidebarStyles{
		appearance:      appearance,
		scheme:          scheme,
		accent:          lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.accent)),
		dim:             lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.dim)),
		treeGuide:       lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.treeGuide)),
		active:          lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.active)).Bold(true),
		selected:        lipgloss.NewStyle().Background(lipgloss.Color(scheme.selectedRowBackgroundRGB.Hex())).Foreground(lipgloss.Color(scheme.selectedForeground)).Bold(true),
		pinned:          lipgloss.NewStyle().Foreground(lipgloss.Color(defaultPinColor)).Bold(true),
		warning:         lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.warning)).Bold(true),
		destructive:     lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.destructive)).Bold(true),
		versionBadge:    lipgloss.NewStyle().Background(lipgloss.Color(scheme.versionBadgeBackground)).Foreground(lipgloss.Color(scheme.versionBadgeForeground)).Bold(true),
		updateIndicator: lipgloss.NewStyle().Background(lipgloss.Color(scheme.updateIndicatorBackground)).Foreground(lipgloss.Color(scheme.updateIndicatorForeground)).Bold(true),
	}
}

func sessionRowStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	style := baseSessionRowStyle(styles, item)
	if strings.TrimSpace(item.PinColor) != "" {
		return style.Foreground(lipgloss.Color(item.PinColor))
	}
	return style
}

func baseSessionRowStyle(styles sidebarStyles, item SessionItem) lipgloss.Style {
	if item.Pinned {
		return styles.pinned.Foreground(lipgloss.Color(pinColor(item)))
	}
	if item.Current {
		return styles.active
	}
	if item.Heat == "" || item.Heat == string(heat.BucketStale) {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(inactiveSessionColorForAppearance(styles.appearance, item.InactiveIntensity)))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(heatColorForAppearance(styles.appearance, item.HeatIntensity)))
}

func pinColor(item SessionItem) string {
	if strings.TrimSpace(item.PinColor) == "" {
		return defaultPinColor
	}
	return item.PinColor
}

func (m SidebarModel) messageStyle(styles sidebarStyles) lipgloss.Style {
	switch m.mode {
	case ModeConfirmDelete:
		return styles.destructive
	case ModeConfirmKill:
		return styles.warning
	default:
		return styles.accent
	}
}

func (m SidebarModel) statusLine() string {
	switch m.mode {
	case ModeSearch:
		return "filter: " + m.filter
	case ModeProject:
		if m.menu.Spec.Filterable && m.menu.Filter != "" {
			return m.menu.Spec.Title + ": " + m.menu.Filter
		}
		return m.menu.Spec.Title
	case ModeCreateNamed:
		return "new session: " + m.createNamedInput
	case ModeCreateCategory:
		return "new category: " + m.createCategoryInput
	case ModeRenameCategory:
		return "rename category: " + m.renameCategoryInput
	case ModeConfirmKill:
		return "confirm kill"
	case ModeConfirmDelete:
		return ""
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
