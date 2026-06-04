package uity

import (
	"fmt"
	"os"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/bnema/tmux-session-sidebar/core/config"
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
	if m.showHelp {
		return []string{
			styles.dim.Render("↵ switch  c create  n layout  / filter"),
			styles.dim.Render(spaceKeySymbol + " pin  J/K move  r rename"),
			styles.dim.Render("x kill  h nums  u update  esc back"),
			styles.dim.Render("? hide"),
		}
	}
	return []string{m.collapsedHelpLine(styles)}
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

func sessionMarkerStyle(styles sidebarStyles, item SessionItem, animationStyle config.AgentAttentionAnimation, animationFrame int, background rgbColor) lipgloss.Style {
	if item.Attention {
		return animatedAttentionMarkerStyle(styles.active, animationStyle, animationFrame, background)
	}
	if item.Current {
		return styles.active
	}
	if item.Pinned {
		return styles.pinned.Foreground(lipgloss.Color(pinColor(item)))
	}
	return sessionRowStyle(styles, item)
}

func pinColor(item SessionItem) string {
	if strings.TrimSpace(item.PinColor) == "" {
		return defaultPinColor
	}
	return item.PinColor
}

func sessionMarker(item SessionItem, animationStyle config.AgentAttentionAnimation, animationFrame int) string {
	if item.Attention {
		return animatedAttentionMarkerSymbol(attentionMarkerSymbol, animationStyle, animationFrame)
	}
	if item.Current {
		return currentMarkerSymbol
	}
	if item.Pinned {
		return pinnedMarkerSymbol
	}
	return " "
}

func (m SidebarModel) statusLine() string {
	switch m.mode {
	case ModeSearch:
		return "filter: " + m.filter
	case ModeProject:
		return "projects: " + m.projectFilter
	case ModeNewItem:
		return "new item"
	case ModeCreateSession:
		return "create session"
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

func (m SidebarModel) renderSessions(styles sidebarStyles) []string {
	visible := m.visibleItems()
	lines := make([]string, 0, len(visible)+1)
	for i, item := range visible {
		badge := "    "
		if item.Slot > 0 {
			badge = fmt.Sprintf("[%s] ", slotLabel(item.Slot))
		}
		lines = append(lines, m.renderSessionRow(styles, item, badge, i == m.cursor))
		if subline := m.renderSessionMetadataSubline(styles, item, badge, i == m.cursor); subline != "" {
			lines = append(lines, subline)
		}
	}
	if len(visible) == 0 {
		lines = append(lines, styles.dim.Render("no sessions"))
	}
	return lines
}

func (m SidebarModel) renderSessionRow(styles sidebarStyles, item SessionItem, badge string, selected bool) string {
	if selected {
		if item.Pinned || item.Attention {
			marker := sessionMarkerStyle(styles, item, m.attentionAnimationStyle, m.attentionAnimationFrame, selectedRowBackgroundRGB).
				Background(lipgloss.Color(selectedRowBackgroundRGB.Hex())).
				Render(sessionMarker(item, m.attentionAnimationStyle, m.attentionAnimationFrame))
			body := styles.selected.Render(fmt.Sprintf(" %s%s", badge, item.Name))
			return marker + body
		}
		return styles.selected.Render(fmt.Sprintf("%s %s%s", sessionMarker(item, m.attentionAnimationStyle, m.attentionAnimationFrame), badge, item.Name))
	}
	marker := sessionMarkerStyle(styles, item, m.attentionAnimationStyle, m.attentionAnimationFrame, defaultAttentionBackgroundRGB).
		Render(sessionMarker(item, m.attentionAnimationStyle, m.attentionAnimationFrame))
	body := sessionRowStyle(styles, item).Render(fmt.Sprintf("%s%s", badge, item.Name))
	return marker + " " + body
}

func (m SidebarModel) renderSessionMetadataSubline(_ sidebarStyles, item SessionItem, badge string, selected bool) string {
	if item.Metadata.Kind == "" {
		return ""
	}
	// Account for outer padding plus the metadata indent; fallback keeps rendering useful before the first WindowSizeMsg.
	width := m.width - metadataSublinePaddingWidth
	if width <= 0 {
		width = metadataSublineFallbackWidth
	}
	subline := RenderMetadataSubline(item.Metadata, MetadataSublineRenderOptions{Icons: m.metadataIconMode, Width: width, Selected: selected, Active: metadataColorActive(item)})
	if subline == "" {
		return ""
	}
	indent := "  " + strings.Repeat(" ", metadataDisplayWidth(badge))
	return indent + subline
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

func (m SidebarModel) renderProjects(styles sidebarStyles) []string {
	visible := m.visibleProjects()
	lines := make([]string, 0, len(visible)+1)
	for i, project := range visible {
		row := fmt.Sprintf("  %-18s", project.Name)
		if i == m.projectCursor {
			row = styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	if len(visible) == 0 {
		lines = append(lines, styles.dim.Render("no projects"))
	}
	return lines
}
