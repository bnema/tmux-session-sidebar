package uity

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSidebarModelSearchFilterRendersTopChipWithoutLiteralLabel(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{}, SidebarOptions{MetadataIconMode: MetadataIconsNerd})
	model.width = 30
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)

	view := stripANSI(model.Render())
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "\uf0b0") || !strings.Contains(lines[0], "b") || !strings.Contains(lines[0], "esc") {
		t.Fatalf("active search filter chip line = %q, want icon, query, and clear hint", lines)
	}
	if strings.Contains(view, "filter:") {
		t.Fatalf("active search render still contains literal filter label: %q", view)
	}
}

func TestSidebarModelCommittedFilterRendersTopChip(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}, Actions{}, SidebarOptions{MetadataIconMode: MetadataIconsNerd})
	model.width = 30
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	lines := strings.Split(stripANSI(model.Render()), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "\uf0b0") || !strings.Contains(lines[0], "b") || !strings.Contains(lines[0], "esc") {
		t.Fatalf("top filter chip line = %q, want icon, query, and clear hint", lines)
	}
	if strings.Contains(stripANSI(model.Render()), "filter:") {
		t.Fatalf("committed filter render still contains literal filter label: %q", stripANSI(model.Render()))
	}
}

func TestSidebarModelCommittedFilterRendersASCIIFallbackChip(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}, {Name: "beta"}}, Actions{}, SidebarOptions{MetadataIconMode: MetadataIconsASCII})
	model.width = 30
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(keyPress("b", 0))
	model = requireSidebarModel(t, updated)
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	line := strings.Split(stripANSI(model.Render()), "\n")[0]
	if !strings.Contains(line, "/") || strings.Contains(line, "\uf0b0") || !strings.Contains(line, "b") || !strings.Contains(line, "esc") {
		t.Fatalf("ASCII filter chip line = %q, want slash fallback, query, and clear hint without Nerd icon", line)
	}
}

func TestSidebarModelFilterChipFitsThirtyColumnSidebar(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{MetadataIconMode: MetadataIconsNerd})
	model.width = 30
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	for _, r := range "ＷＩＤＥ-super-long-filter-content" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	line := strings.Split(stripANSI(model.Render()), "\n")[0]
	if got := metadataDisplayWidth(line); got > 30 {
		t.Fatalf("filter chip display width = %d for %q, want <= 30", got, line)
	}
	if !strings.Contains(line, "…") {
		t.Fatalf("filter chip line = %q, want ellipsized long filter", line)
	}
}

func TestSidebarModelASCIIFilterChipFitsVeryNarrowSidebar(t *testing.T) {
	model := newTestSidebarModelWithOptions([]SessionItem{{Name: "alpha"}}, Actions{}, SidebarOptions{MetadataIconMode: MetadataIconsASCII})
	model.width = 5
	updated, _ := model.Update(keyPress("/", 0))
	model = requireSidebarModel(t, updated)
	for _, r := range "abcdef" {
		updated, _ = model.Update(keyPress(string(r), 0))
		model = requireSidebarModel(t, updated)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = requireSidebarModel(t, updated)

	line := strings.Split(stripANSI(model.Render()), "\n")[0]
	if got := metadataDisplayWidth(line); got > model.width {
		t.Fatalf("ASCII filter chip display width = %d for %q, want <= %d", got, line, model.width)
	}
}
