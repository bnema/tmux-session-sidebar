package uity

import (
	"fmt"
	"strings"
)

func (m SidebarModel) renderBottomSheet(content string, sheet BottomSheet) string {
	width := m.width
	if width <= 0 {
		width = max(metadataDisplayWidth(content), 1)
	}
	height := m.height
	if height <= 0 {
		height = max(1, len(strings.Split(content, "\n")))
	}
	return sheet.RenderOverlay(content, width, height)
}

func (m SidebarModel) renderMenuContent(styles sidebarStyles) string {
	visible := m.visibleProjects()
	if len(visible) == 0 {
		return styles.dim.Render("no items")
	}
	lines := make([]string, 0, len(visible))
	for i, item := range visible {
		cursor := "  "
		if i == m.projectCursor {
			cursor = "> "
		}
		line := fmt.Sprintf("%s%s", cursor, item.Name)
		if i == m.projectCursor {
			line = styles.selected.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
