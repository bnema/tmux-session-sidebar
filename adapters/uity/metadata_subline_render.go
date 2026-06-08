package uity

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

type MetadataSublineRenderOptions struct {
	Icons    MetadataIconMode
	Width    int
	Selected bool
	Active   bool
}

func RenderMetadataSubline(meta SessionMetadataSubline, options MetadataSublineRenderOptions) string {
	parts := formatMetadataSublineParts(meta, MetadataSublineOptions{Icons: options.Icons, Width: options.Width})
	if len(parts) == 0 {
		return ""
	}
	return renderMetadataParts(parts, options.Selected, options.Active)
}

func formatMetadataSublineParts(meta SessionMetadataSubline, options MetadataSublineOptions) []metadataPart {
	width := options.Width
	if width <= 0 {
		width = metadataSublineFallbackWidth
	}
	if options.Icons == "" {
		options.Icons = MetadataIconsNerd
	}
	if meta.Kind == MetadataKindGit {
		return formatGitMetadataSublineParts(meta, options.Icons, width)
	}
	text := FormatMetadataSubline(meta, options)
	if text == "" {
		return nil
	}
	return []metadataPart{{Text: text, Role: metadataPartBase}}
}

func renderMetadataParts(parts []metadataPart, selected bool, active bool) string {
	if !active {
		return metadataInactiveSublineStyle(selected).Render(metadataPartText(parts))
	}
	base := metadataSublineStyle()
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(base.Render(" "))
		}
		b.WriteString(renderMetadataPart(part, selected))
	}
	return b.String()
}

func metadataSublineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
}

func metadataInactiveSublineStyle(selected bool) lipgloss.Style {
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(selectedInactiveMetadataRGB.Hex()))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(inactiveMetadataRGB.Hex()))
}

func renderMetadataPart(part metadataPart, selected bool) string {
	if part.Role == metadataPartCompare {
		return renderCompareMetadataPart(part.Text, selected)
	}
	if part.Role == metadataPartUpstream {
		return renderUpstreamMetadataPart(part.Text, selected)
	}
	return metadataPartStyle(part.Role, selected).Render(part.Text)
}

func renderCompareMetadataPart(text string, selected bool) string {
	before, after, ok := strings.Cut(text, "/")
	if !ok {
		return metadataPartStyle(metadataPartCompare, selected).Render(text)
	}
	compare := metadataPartStyle(metadataPartCompare, selected)
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	if selected {
		separator = lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
	}
	return compare.Render(before) + separator.Render("/") + compare.Render(after)
}

func renderUpstreamMetadataPart(text string, selected bool) string {
	behindIndex := strings.Index(text, MetadataGitBehind)
	if behindIndex < 0 {
		return metadataPartStyle(metadataPartAhead, selected).Render(text)
	}
	if behindIndex == 0 {
		return metadataPartStyle(metadataPartBehind, selected).Render(text)
	}
	return metadataPartStyle(metadataPartAhead, selected).Render(text[:behindIndex]) + metadataPartStyle(metadataPartBehind, selected).Render(text[behindIndex:])
}

func metadataPartStyle(role metadataPartRole, selected bool) lipgloss.Style {
	colors := metadataActivePartColors(selected)
	switch role {
	case metadataPartCompare:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.compare))
	case metadataPartAhead:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.ahead))
	case metadataPartBehind, metadataPartConflict:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.behind))
	case metadataPartStaged:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.staged))
	case metadataPartUnstaged:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colors.unstaged))
	default:
		if selected {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
		}
		return metadataSublineStyle()
	}
}

type metadataColors struct {
	compare  string
	ahead    string
	behind   string
	staged   string
	unstaged string
}

func metadataActivePartColors(selected bool) metadataColors {
	if selected {
		return metadataColors{compare: "#7dd3fc", ahead: "#86efac", behind: "#f87171", staged: "#93c5fd", unstaged: "#e4d987"}
	}
	return metadataColors{compare: "#38bdf8", ahead: "#4ade80", behind: "#f87171", staged: "#60a5fa", unstaged: "#d6c86f"}
}
