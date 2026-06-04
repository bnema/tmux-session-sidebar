package uity

import "strings"

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
