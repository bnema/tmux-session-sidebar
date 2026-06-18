package uity

// StatusBar anchors footer lines to the bottom of the available terminal height.
// It lives in the terminal UI adapter because terminal geometry is a delivery
// concern, not domain behavior.
type StatusBar struct {
	Lines  []string
	Height int
}

func (s StatusBar) RenderBelow(content []string) []string {
	lines := append([]string{}, content...)
	if len(s.Lines) == 0 {
		return lines
	}
	if s.Height > 0 {
		maxContentLines := max(s.Height-len(s.Lines), 0)
		if len(lines) > maxContentLines {
			lines = lines[:maxContentLines]
		}
		for gap := s.Height - len(lines) - len(s.Lines); gap > 0; gap-- {
			lines = append(lines, "")
		}
	}
	return append(lines, s.Lines...)
}
