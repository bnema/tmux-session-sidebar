package uity

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

type treeRenderer struct {
	styles           sidebarStyles
	width            int
	metadataIconMode MetadataIconMode
	animationStyle   config.AgentAttentionAnimation
	animationFrame   int
	cursor           int
}

func (r treeRenderer) Render(items []TreeItem) []string {
	if len(items) == 0 {
		return []string{r.styles.dim.Render("no sessions")}
	}
	lines := make([]string, 0, len(items)*2)
	selectableIndex := 0
	for _, item := range items {
		selectable := isSelectableTreeKind(item.Kind)
		selected := selectable && selectableIndex == r.cursor
		switch item.Kind {
		case TreeRowCategory:
			lines = append(lines, r.renderCategory(item, selected))
			if selectable {
				selectableIndex++
			}
		case TreeRowSession:
			lines = append(lines, r.renderSession(item, selected))
			if subline := r.renderMetadata(item, selected); subline != "" {
				lines = append(lines, subline)
			}
			if selectable {
				selectableIndex++
			}
		case TreeRowMore:
			lines = append(lines, r.renderMore(item, selected))
			if selectable {
				selectableIndex++
			}
		case TreeRowSeparator:
			separator := r.separatorLine()
			line := r.styles.dim.Render(separator)
			if selected {
				line = r.styles.selected.Render(separator)
			}
			lines = append(lines, line)
			if selectable {
				selectableIndex++
			}
		case TreeRowSpacer:
			line := ""
			if selected {
				line = r.styles.selected.Render(" ")
			}
			lines = append(lines, line)
			if selectable {
				selectableIndex++
			}
		}
	}
	return lines
}

func (r treeRenderer) separatorLine() string {
	width := r.width
	if width <= 0 {
		width = 24
	}
	return strings.Repeat("─", width)
}

func (r treeRenderer) renderCategory(item TreeItem, selected bool) string {
	marker := "▸"
	if item.CategoryOpen {
		marker = "▾"
	}
	name := strings.TrimSpace(item.CategoryName)
	if name == "" {
		name = "Default"
	}
	line := marker + " " + name
	if selected {
		style := r.styles.selected
		if strings.TrimSpace(item.Color) != "" {
			style = style.Foreground(lipgloss.Color(item.Color))
		}
		return style.Render(line)
	}
	if strings.TrimSpace(item.Color) != "" {
		return r.styles.accent.Foreground(lipgloss.Color(item.Color)).Render(line)
	}
	return r.styles.accent.Render(line)
}

func (r treeRenderer) renderSession(item TreeItem, selected bool) string {
	session := item.Session
	branch := r.styles.treeGuide.Render(treeBranch(item))
	currentMarker := currentSessionMarker(session)
	slot := slotPrefix(item.Slot)
	marker := treeSessionMarker(session)
	name := r.fitSessionName(item, currentMarker, slot, marker)
	bodyText := sessionBodyText(slot, marker, name)
	if currentMarker != "" {
		bodyText = strings.TrimPrefix(bodyText, " ")
	}
	if selected {
		style := r.styles.selected
		if strings.TrimSpace(session.PinColor) != "" {
			style = style.Foreground(lipgloss.Color(session.PinColor))
		}
		body := style.Render(bodyText)
		return branch + currentMarker + body + r.renderAttention(session, true)
	}
	body := sessionRowStyle(r.styles, session).Render(bodyText)
	return branch + currentMarker + body + r.renderAttention(session, false)
}

func (r treeRenderer) fitSessionName(item TreeItem, currentMarker string, slot string, marker string) string {
	name := sanitizeSessionName(item.Session.Name)
	width := r.width
	if width <= 0 {
		return name
	}
	budget := width - 2 // account for sidebar horizontal padding added after tree rendering.
	budget -= metadataDisplayWidth(treeBranch(item))
	budget -= metadataDisplayWidth(currentMarker)
	budget -= sessionBodyPrefixWidth(slot, marker, currentMarker != "")
	if item.Session.Attention {
		budget -= metadataDisplayWidth(" ") + metadataDisplayWidth(attentionMarkerSymbol)
	}
	if budget <= 0 {
		return ""
	}
	return fitMetadataText(name, budget, r.metadataIconMode)
}

func sessionBodyPrefixWidth(slot string, marker string, current bool) int {
	width := 1 // leading space before the row body.
	if current {
		width--
	}
	if strings.TrimSpace(slot) != "" {
		width += metadataDisplayWidth(strings.TrimSpace(slot)) + 1
	}
	if strings.TrimSpace(marker) != "" {
		width += metadataDisplayWidth(strings.TrimSpace(marker)) + 1
	}
	return max(width, 0)
}

func (r treeRenderer) renderMore(item TreeItem, selected bool) string {
	branch := r.styles.treeGuide.Render(treeBranch(item))
	label := "[show less]"
	if !item.MoreExpanded {
		label = fmt.Sprintf("[show %d more]", item.MoreCount)
	}
	bodyText := " " + label
	if selected {
		return branch + r.styles.selected.Italic(true).Render(bodyText)
	}
	return branch + r.styles.dim.Italic(true).Render(bodyText)
}

func (r treeRenderer) renderAttention(session SessionItem, selected bool) string {
	if !session.Attention {
		return ""
	}
	style := animatedAttentionMarkerStyle(r.styles.active, r.animationStyle, r.animationFrame, defaultAttentionBackgroundRGB)
	if selected {
		style = style.Background(lipgloss.Color(selectedRowBackgroundRGB.Hex()))
	}
	return style.Render(" " + animatedAttentionMarkerSymbol(attentionMarkerSymbol, r.animationStyle, r.animationFrame))
}

func (r treeRenderer) renderMetadata(item TreeItem, selected bool) string {
	if !item.ShowMetadata || item.Session.Metadata.Kind == "" {
		return ""
	}
	width := r.width
	if width <= 0 {
		width = metadataSublineSidebarFallbackWidth
	}
	width -= metadataSublinePaddingWidth
	if width <= 0 {
		return ""
	}
	prefix := treeMetadataPrefix(item)
	width -= metadataDisplayWidth(prefix)
	if width <= 0 {
		return ""
	}
	subline := RenderMetadataSubline(item.Session.Metadata, MetadataSublineRenderOptions{Icons: r.metadataIconMode, Width: width, Selected: selected, Active: metadataColorActive(item.Session)})
	if subline == "" {
		return ""
	}
	return r.styles.treeGuide.Render(prefix) + subline
}

func treeBranch(item TreeItem) string {
	if item.Depth <= 0 {
		return ""
	}
	if item.LastChild {
		return "└─"
	}
	return "├─"
}

func treeMetadataPrefix(item TreeItem) string {
	if item.Depth <= 0 {
		return ""
	}
	indent := metadataNameIndent(item)
	if !item.LastChild && indent > 0 {
		return "│" + strings.Repeat(" ", indent-1)
	}
	return strings.Repeat(" ", indent)
}

func metadataNameIndent(item TreeItem) int {
	indent := metadataDisplayWidth(treeBranch(item))
	if item.Session.Current {
		indent += metadataDisplayWidth("┃")
	} else {
		indent++
	}
	if slot := slotPrefix(item.Slot); strings.TrimSpace(slot) != "" {
		indent += metadataDisplayWidth(strings.TrimSpace(slot)) + 1
	}
	if marker := treeSessionMarker(item.Session); strings.TrimSpace(marker) != "" {
		indent += metadataDisplayWidth(strings.TrimSpace(marker)) + 1
	}
	return max(indent, 0)
}

func currentSessionMarker(item SessionItem) string {
	if !item.Current {
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(selectedRowBackgroundRGB.Hex())).Render("┃")
}

func treeSessionMarker(item SessionItem) string {
	if item.Pinned {
		return pinnedMarkerSymbol
	}
	return ""
}

func sessionBodyText(slot string, marker string, name string) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(slot) != "" {
		parts = append(parts, strings.TrimSpace(slot))
	}
	if strings.TrimSpace(marker) != "" {
		parts = append(parts, strings.TrimSpace(marker))
	}
	parts = append(parts, name)
	return " " + strings.Join(parts, " ")
}

func slotPrefix(slot int) string {
	if slot <= 0 {
		return ""
	}
	return fmt.Sprintf("%d ", slot)
}
