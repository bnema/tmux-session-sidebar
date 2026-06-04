package uity

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

const defaultBottomSheetHeight = 7

type bottomSheet struct {
	Title   string
	Content string
	Footer  string
	Height  int
}

func (s bottomSheet) Render(width int) string {
	return s.render(width, 0)
}

func (s bottomSheet) render(width int, maxHeight int) string {
	if width <= 0 {
		width = metadataSublineFallbackWidth
	}
	innerWidth := max(width, 1)
	titleLines := []string{}
	if strings.TrimSpace(s.Title) != "" {
		title := fitMetadataText(strings.TrimSpace(s.Title), innerWidth, MetadataIconsASCII)
		titleLines = append(titleLines, bottomSheetTitleStyle().Width(innerWidth).Align(lipgloss.Center).Render(title))
	}
	contentLines := []string{}
	if s.Content != "" {
		contentLines = fitBlockWidth(s.Content, innerWidth)
	}
	footerLines := []string{}
	if s.Footer != "" {
		footerLines = append(footerLines, bottomSheetFooterStyle().Render(fitMetadataText(s.Footer, innerWidth, MetadataIconsASCII)))
	}
	bodyLines := boundedBottomSheetLines(titleLines, contentLines, footerLines, maxHeight)
	separator := bottomSheetSeparator(width)
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, separator)
	lines = append(lines, bodyLines...)
	lines = append(lines, separator)
	return strings.Join(lines, "\n")
}

func boundedBottomSheetLines(titleLines []string, contentLines []string, footerLines []string, maxHeight int) []string {
	if maxHeight <= 0 {
		return append(append(titleLines, contentLines...), footerLines...)
	}
	innerHeight := max(maxHeight-2, 0)
	if innerHeight == 0 {
		return nil
	}
	fixedLines := len(titleLines) + len(footerLines)
	contentHeight := max(innerHeight-fixedLines, 0)
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	lines := append(append(titleLines, contentLines...), footerLines...)
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	return lines
}

func (s bottomSheet) RenderOverlay(base string, width int, height int) string {
	if width <= 0 {
		width = max(lipgloss.Width(base), 1)
	}
	if height <= 0 {
		height = max(lipgloss.Height(base), 1)
	}
	maxHeight := s.Height
	if maxHeight <= 0 {
		maxHeight = min(defaultBottomSheetHeight, height)
	}
	if maxHeight > height {
		maxHeight = height
	}
	sheet := s.render(width, maxHeight)
	sheetHeight := lipgloss.Height(sheet)
	canvas := lipgloss.NewCanvas(width, height)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(clipBlock(base, height)),
		lipgloss.NewLayer(sheet).X(0).Y(max(height-sheetHeight, 0)).Z(1),
	)
	canvas.Compose(compositor)
	return canvas.Render()
}

func bottomSheetSeparator(width int) string {
	return strings.Repeat("─", max(width, 1))
}

func bottomSheetTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#e0f2fe")).Bold(true)
}

func bottomSheetFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
}

func fitBlockWidth(value string, width int) []string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = fitBottomSheetContentLine(line, width)
	}
	return lines
}

func fitBottomSheetContentLine(line string, width int) string {
	if width <= 0 || metadataDisplayWidth(line) <= width {
		return line
	}
	ellipsis := "..."
	return trimDisplayRight(line, max(width-metadataDisplayWidth(ellipsis), 0)) + ellipsis
}
