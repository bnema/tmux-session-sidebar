package uity

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

const defaultPinColor = "#facc15"
const selectedPinColorAccent = "#065f46"

var pinColorPalette = generatePinColorPalette()

func generatePinColorPalette() []string {
	// Columns move around the hue wheel; rows move from soft pastel to richer
	// vivid tones. This gives the picker a predictable visual hierarchy instead
	// of a random-looking scatter of colors.
	hues := []float64{42, 145, 205, 315}
	tiers := []struct {
		saturation float64
		lightness  float64
	}{
		{saturation: 0.55, lightness: 0.84},
		{saturation: 0.68, lightness: 0.74},
		{saturation: 0.78, lightness: 0.62},
		{saturation: 0.86, lightness: 0.48},
	}
	palette := make([]string, 0, len(hues)*len(tiers))
	for _, tier := range tiers {
		for _, hue := range hues {
			palette = append(palette, hslHex(hue, tier.saturation, tier.lightness))
		}
	}
	return palette
}

type PinColorPicker struct {
	Cursor int
}

func (p PinColorPicker) SelectedColor() string {
	if p.Cursor < 0 || p.Cursor >= len(pinColorPalette) {
		return defaultPinColor
	}
	return pinColorPalette[p.Cursor]
}

func (p PinColorPicker) Move(delta int) int {
	if len(pinColorPalette) == 0 {
		return 0
	}
	return (p.Cursor + delta + len(pinColorPalette)) % len(pinColorPalette)
}

func (p PinColorPicker) MoveDelta(msg tea.KeyPressMsg) (int, bool) {
	columns := p.columns()
	key := msg.Key()
	switch key.Code {
	case tea.KeyRight:
		return 1, true
	case tea.KeyLeft:
		return -1, true
	case tea.KeyDown:
		return columns, true
	case tea.KeyUp:
		return -columns, true
	}
	switch key.Text {
	case "l", "L":
		return 1, true
	case "h", "H":
		return -1, true
	case "j", "J":
		return columns, true
	case "k", "K":
		return -columns, true
	default:
		return 0, false
	}
}

func (p PinColorPicker) RenderOverlay(content string, width int, height int) string {
	return p.RenderOverlayAt(content, width, height, pinColorOverlayY(height, lipgloss.Height(p.Render())))
}

func (p PinColorPicker) RenderOverlayAt(content string, width int, height int, y int) string {
	pane := p.Render()
	if width <= 0 {
		width = max(lipgloss.Width(content), lipgloss.Width(pane))
	}
	if height <= 0 {
		height = max(lipgloss.Height(content), lipgloss.Height(pane))
	}
	y = min(max(y, 0), max(height-lipgloss.Height(pane), 0))
	canvas := lipgloss.NewCanvas(width, height)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(clipBlock(content, height)),
		lipgloss.NewLayer(pane).X(max((width-lipgloss.Width(pane))/2, 0)).Y(y).Z(1),
	)
	canvas.Compose(compositor)
	return canvas.Render()
}

func (p PinColorPicker) Render() string {
	styles := newPinColorPickerStyles()
	columns := p.columns()
	rows := make([]string, 0, (len(pinColorPalette)+columns-1)/columns)
	for start := 0; start < len(pinColorPalette); start += columns {
		cells := make([]string, 0, columns)
		for i := 0; i < columns && start+i < len(pinColorPalette); i++ {
			index := start + i
			cells = append(cells, p.renderSwatch(styles, index))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("colorize"),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		styles.hint.Render("↵/sp ok esc"),
	)
	return styles.box.Render(body)
}

func (p PinColorPicker) columns() int {
	return 4
}

func (p PinColorPicker) renderSwatch(styles pinColorPickerStyles, index int) string {
	color := lipgloss.Color(pinColorPalette[index])
	style := styles.swatch.Foreground(color)
	if index == p.Cursor {
		return style.Background(lipgloss.Color(selectedPinColorAccent)).Render("▄▄▄▄\n▀▀▀▀")
	}
	return style.Render("████\n████")
}

type pinColorPickerStyles struct {
	box    lipgloss.Style
	title  lipgloss.Style
	hint   lipgloss.Style
	swatch lipgloss.Style
}

func newPinColorPickerStyles() pinColorPickerStyles {
	background := lipgloss.Color("#0f172a")
	return pinColorPickerStyles{
		box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(selectedPinColorAccent)).Background(background).Padding(0, 1),
		title:  lipgloss.NewStyle().Foreground(lipgloss.Color("#e0f2fe")).Background(background).Bold(true),
		hint:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Background(background),
		swatch: lipgloss.NewStyle().Background(background),
	}
}

func pinColorOverlayY(height int, paneHeight int) int {
	return min(1, max(height-paneHeight, 0))
}

func clipBlock(content string, height int) string {
	if height <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= height {
		return content
	}
	return strings.Join(lines[:height], "\n")
}
