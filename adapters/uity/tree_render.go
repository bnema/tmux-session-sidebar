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
		selected := selectableIndex == r.cursor
		switch item.Kind {
		case TreeRowCategory:
			lines = append(lines, r.renderCategory(item, selected))
			selectableIndex++
		case TreeRowSession:
			lines = append(lines, r.renderSession(item, selected))
			if subline := r.renderMetadata(item, selected); subline != "" {
				lines = append(lines, subline)
			}
			selectableIndex++
		case TreeRowSeparator:
			line := r.styles.dim.Render("────────────────────────")
			if selected {
				line = r.styles.selected.Render("────────────────────────")
			}
			lines = append(lines, line)
			selectableIndex++
		case TreeRowSpacer:
			line := ""
			if selected {
				line = r.styles.selected.Render(" ")
			}
			lines = append(lines, line)
			selectableIndex++
		}
	}
	return lines
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
		return r.styles.selected.Render(line)
	}
	return r.styles.accent.Render(line)
}

func (r treeRenderer) renderSession(item TreeItem, selected bool) string {
	session := item.Session
	branch := item.Branch
	if branch == "" {
		branch = "├─"
	}
	slot := slotPrefix(item.Slot)
	marker := treeSessionMarker(session)
	name := sanitizeSessionName(session.Name)
	bodyText := fmt.Sprintf("%s %s%s %s", branch, slot, marker, name)
	if selected {
		body := r.styles.selected.Render(bodyText)
		return body + r.renderAttention(session, true)
	}
	body := sessionRowStyle(r.styles, session).Render(bodyText)
	return body + r.renderAttention(session, false)
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
	width := r.width - metadataSublinePaddingWidth
	if width <= 0 {
		width = metadataSublineFallbackWidth
	}
	prefix := item.MetadataPrefix
	if prefix == "" {
		prefix = "│  "
	}
	width -= metadataDisplayWidth(prefix)
	if width <= 0 {
		return ""
	}
	subline := RenderMetadataSubline(item.Session.Metadata, MetadataSublineRenderOptions{Icons: r.metadataIconMode, Width: width, Selected: selected, Active: metadataColorActive(item.Session)})
	if subline == "" {
		return ""
	}
	return r.styles.dim.Render(prefix) + subline
}

func treeSessionMarker(item SessionItem) string {
	if item.Current {
		return currentMarkerSymbol
	}
	if item.Pinned {
		return pinnedMarkerSymbol
	}
	return " "
}

func slotPrefix(slot int) string {
	if slot <= 0 || slot > 10 {
		return ""
	}
	if slot == 10 {
		return "0 "
	}
	return fmt.Sprintf("%d ", slot)
}
