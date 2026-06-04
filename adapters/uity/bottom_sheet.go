package uity

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

const defaultBottomSheetHeight = 7

type BottomSheet struct {
	Title   string
	Content string
	Footer  string
	Height  int
}

func (s BottomSheet) Render(width int) string {
	return s.render(width, 0)
}

func (s BottomSheet) render(width int, maxHeight int) string {
	if width <= 0 {
		width = metadataSublineFallbackWidth
	}
	innerWidth := max(width-2, 1)
	titleLines := []string{}
	if strings.TrimSpace(s.Title) != "" {
		titleLines = append(titleLines, bottomSheetTitleStyle().Render(fitMetadataText(s.Title, innerWidth, MetadataIconsASCII)))
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
	content := strings.Join(bodyLines, "\n")
	return bottomSheetBoxStyle(width).Render(content)
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

func (s BottomSheet) RenderOverlay(base string, width int, height int) string {
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

func bottomSheetBoxStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(max(width-2, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Background(lipgloss.Color("#0f172a")).
		Padding(0, 0)
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
		lines[i] = fitMetadataText(line, width, MetadataIconsASCII)
	}
	return lines
}
