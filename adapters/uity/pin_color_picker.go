package uity

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/bnema/tmux-session-sidebar/internal/core/config"
)

const defaultPinColor = "#facc15"
const pinColorResetLabel = "auto"

var pinColorPalette = []string{
	defaultPinColor,
	"#f97316",
	"#ef4444",
	"#ec4899",
	"#a855f7",
	"#6366f1",
	"#3b82f6",
	"#06b6d4",
	"#14b8a6",
	"#22c55e",
	"#84cc16",
}

type PinColorPicker struct {
	Cursor     int
	Appearance config.ColorSchemeAppearance
}

func (p PinColorPicker) SelectedColor() string {
	if p.Cursor == p.resetIndex() {
		return ""
	}
	if p.Cursor < 0 || p.Cursor >= len(pinColorPalette) {
		return defaultPinColor
	}
	return pinColorPalette[p.Cursor]
}

func (p PinColorPicker) Move(delta int) int {
	return (p.Cursor + delta + p.optionCount()) % p.optionCount()
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
	styles := newPinColorPickerStyles(p.Appearance)
	columns := p.columns()
	rows := make([]string, 0, (p.optionCount()+columns-1)/columns)
	for start := 0; start < p.optionCount(); start += columns {
		cells := make([]string, 0, columns)
		for i := 0; i < columns && start+i < p.optionCount(); i++ {
			index := start + i
			cells = append(cells, p.renderOption(styles, index))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("colorize"),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		styles.hint.Render("↵/sp apply\nesc cancel"),
	)
	return styles.box.Render(body)
}

func (p PinColorPicker) columns() int {
	return 4
}

func (p PinColorPicker) optionCount() int {
	return len(pinColorPalette) + 1
}

func (p PinColorPicker) resetIndex() int {
	return len(pinColorPalette)
}

func (p PinColorPicker) renderOption(styles pinColorPickerStyles, index int) string {
	if index == p.resetIndex() {
		return p.renderResetOption(styles)
	}
	return p.renderSwatch(styles, index)
}

func (p PinColorPicker) renderSwatch(styles pinColorPickerStyles, index int) string {
	color := lipgloss.Color(pinColorPalette[index])
	style := styles.swatch.Foreground(color)
	if index == p.Cursor {
		return style.Background(lipgloss.Color(styles.selectedBackground)).Render("▄▄▄▄\n▀▀▀▀")
	}
	return style.Render("████\n████")
}

func (p PinColorPicker) renderResetOption(styles pinColorPickerStyles) string {
	style := styles.reset
	if p.Cursor == p.resetIndex() {
		style = style.Background(lipgloss.Color(styles.selectedBackground)).Foreground(lipgloss.Color(styles.selectedForeground)).Bold(true)
	}
	return style.Render("────\n" + pinColorResetLabel)
}

type pinColorPickerStyles struct {
	box                lipgloss.Style
	title              lipgloss.Style
	hint               lipgloss.Style
	swatch             lipgloss.Style
	reset              lipgloss.Style
	selectedBackground string
	selectedForeground string
}

func newPinColorPickerStyles(appearance config.ColorSchemeAppearance) pinColorPickerStyles {
	scheme := colorScheme(appearance)
	background := lipgloss.Color(scheme.pinPickerBackground)
	return pinColorPickerStyles{
		box:                lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(scheme.pinPickerBorder)).Background(background).Padding(0, 1),
		title:              lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.pinPickerTitle)).Background(background).Bold(true),
		hint:               lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.pinPickerHint)).Background(background),
		swatch:             lipgloss.NewStyle().Background(background),
		reset:              lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.pinPickerHint)).Background(background),
		selectedBackground: scheme.selectedRowBackgroundRGB.Hex(),
		selectedForeground: scheme.selectedForeground,
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
