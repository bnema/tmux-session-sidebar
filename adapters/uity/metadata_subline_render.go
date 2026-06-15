package uity

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/bnema/tmux-session-sidebar/core/config"
)

type MetadataSublineRenderOptions struct {
	Icons             MetadataIconMode
	Width             int
	Selected          bool
	Active            bool
	InactiveIntensity float64
	Appearance        config.ColorSchemeAppearance
}

func RenderMetadataSubline(meta SessionMetadataSubline, options MetadataSublineRenderOptions) string {
	parts := formatMetadataSublineParts(meta, MetadataSublineOptions{Icons: options.Icons, Width: options.Width})
	if len(parts) == 0 {
		return ""
	}
	return renderMetadataParts(parts, options)
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

func renderMetadataParts(parts []metadataPart, options MetadataSublineRenderOptions) string {
	if !options.Active {
		return metadataInactiveSublineStyle(options.Appearance, options.Selected, options.InactiveIntensity).Render(metadataPartText(parts))
	}
	base := metadataSublineStyle(options.Appearance)
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(base.Render(" "))
		}
		b.WriteString(renderMetadataPart(part, options.Appearance, options.Selected))
	}
	return b.String()
}

func metadataSublineStyle(appearance config.ColorSchemeAppearance) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorScheme(appearance).metadataBase))
}

func metadataInactiveSublineStyle(appearance config.ColorSchemeAppearance, selected bool, intensity float64) lipgloss.Style {
	scheme := colorScheme(appearance)
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.selectedInactiveMetadataRGB.Hex()))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(inactiveMetadataColorForAppearance(appearance, intensity)))
}

func renderMetadataPart(part metadataPart, appearance config.ColorSchemeAppearance, selected bool) string {
	if part.Role == metadataPartCompare {
		return renderCompareMetadataPart(part.Text, appearance, selected)
	}
	if part.Role == metadataPartUpstream {
		return renderUpstreamMetadataPart(part.Text, appearance, selected)
	}
	return metadataPartStyle(part.Role, appearance, selected).Render(part.Text)
}

func renderCompareMetadataPart(text string, appearance config.ColorSchemeAppearance, selected bool) string {
	before, after, ok := strings.Cut(text, "/")
	if !ok {
		return metadataPartStyle(metadataPartCompare, appearance, selected).Render(text)
	}
	compare := metadataPartStyle(metadataPartCompare, appearance, selected)
	scheme := colorScheme(appearance)
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.metadataSeparator))
	if selected {
		separator = lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.metadataSelectedSeparator))
	}
	return compare.Render(before) + separator.Render("/") + compare.Render(after)
}

func renderUpstreamMetadataPart(text string, appearance config.ColorSchemeAppearance, selected bool) string {
	behindIndex := strings.Index(text, MetadataGitBehind)
	if behindIndex < 0 {
		return metadataPartStyle(metadataPartAhead, appearance, selected).Render(text)
	}
	if behindIndex == 0 {
		return metadataPartStyle(metadataPartBehind, appearance, selected).Render(text)
	}
	return metadataPartStyle(metadataPartAhead, appearance, selected).Render(text[:behindIndex]) + metadataPartStyle(metadataPartBehind, appearance, selected).Render(text[behindIndex:])
}

func metadataPartStyle(role metadataPartRole, appearance config.ColorSchemeAppearance, selected bool) lipgloss.Style {
	colors := metadataActivePartColors(appearance, selected)
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
			return lipgloss.NewStyle().Foreground(lipgloss.Color(colorScheme(appearance).metadataSelectedMuted))
		}
		return metadataSublineStyle(appearance)
	}
}

type metadataColors struct {
	compare  string
	ahead    string
	behind   string
	staged   string
	unstaged string
}

func metadataActivePartColors(appearance config.ColorSchemeAppearance, selected bool) metadataColors {
	scheme := colorScheme(appearance)
	if selected {
		return scheme.metadataSelectedActive
	}
	return scheme.metadataActive
}
