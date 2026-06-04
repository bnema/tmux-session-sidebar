package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/adapters/storefs"
	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	sidebarlayout "github.com/bnema/tmux-session-sidebar/core/sidebar"
	"github.com/bnema/tmux-session-sidebar/ports"
)

func loadSidebarTreeItemsWithConfig(ctx context.Context, cfg ports.ConfigSnapshot) ([]uity.TreeItem, error) {
	current, err := tmux(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("getting current tmux session: %w", err)
	}
	views, err := runtimeService().SessionViews(ctx)
	if err != nil {
		return nil, err
	}
	persisted, err := loadSidebarState(ctx)
	if err != nil {
		return nil, err
	}
	_, byName := sessionItemsFromState(strings.TrimSpace(current), views, persisted, cfg)
	layout := coreLayoutFromPersisted(persisted.SidebarLayout)
	live := sessionNames(sessions.FilterVisible(views, true))
	layout = sidebarlayout.EnsureLayout(layout, live, persisted.SessionOrder)
	selection := sidebarSelectionForCurrent(layout, strings.TrimSpace(current))
	rows := sidebarlayout.Flatten(layout, selection, persistedShowNumeric(persisted))
	activeCategoryID := sidebarlayout.ActiveCategoryID(layout, selection)
	tree := make([]uity.TreeItem, 0, len(rows))
	for _, row := range rows {
		item := uity.TreeItem{ID: row.ItemID, CategoryID: row.CategoryID, CategoryName: row.CategoryName, CategoryOpen: row.CategoryOpen, Slot: row.Slot, Depth: row.Depth, LastChild: row.LastChild}
		switch row.Kind {
		case sidebarlayout.RowKindCategory:
			item.Kind = uity.TreeRowCategory
		case sidebarlayout.RowKindSeparator:
			item.Kind = uity.TreeRowSeparator
		case sidebarlayout.RowKindSpacer:
			item.Kind = uity.TreeRowSpacer
		case sidebarlayout.RowKindSession:
			item.Kind = uity.TreeRowSession
			item.Session = byName[row.Session]
			item.ShowMetadata = cfg.MetadataSublineEnabled && (cfg.MetadataInactiveEnabled || row.CategoryID == activeCategoryID)
			if item.ShowMetadata && item.Session.Metadata.Kind == "" {
				item.Session.Metadata = uity.SessionMetadataSubline{Kind: uity.MetadataKindLoading, SessionName: row.Session}
			}
		}
		tree = append(tree, item)
	}
	return tree, nil
}

func sessionItemsFromState(current string, views []sessions.View, persisted ports.PersistedState, cfg ports.ConfigSnapshot) ([]uity.SessionItem, map[string]uity.SessionItem) {
	heatStates := decodePersistedHeat(persisted.Heat)
	attentionStates := attentionStateMap(persisted.AgentAttention)
	now := time.Now().UTC()
	names := sessions.ApplyOrder(sessionNames(sessions.FilterVisible(views, true)), persisted.SessionOrder)
	pinned := pinnedSessionSet(persisted.PinnedSessions)
	viewsByName := make(map[string]sessions.View, len(views))
	for _, view := range views {
		viewsByName[view.Name] = view
	}
	heatDisplays := heatDisplaysForNames(names, heatStates, now, cfg)
	items := make([]uity.SessionItem, 0, len(names))
	byName := make(map[string]uity.SessionItem, len(names))
	for _, name := range names {
		_, isPinned := pinned[name]
		item := uity.SessionItem{Name: name, Current: name == current, Pinned: isPinned, PinColor: persisted.PinColors[name]}
		if display, ok := heatDisplays[name]; ok && cfg.HeatColorsEnabled {
			item.Heat = string(display.Bucket)
			item.HeatIntensity = display.Intensity
		}
		if cfg.AgentAttentionEnabled {
			if state, ok := attentionStateForSession(attentionStates, viewsByName[name]); ok {
				item.Attention = state.Attention
			}
		}
		if cfg.MetadataSublineEnabled {
			if metadata, ok := persisted.Metadata[name]; ok {
				item.Metadata = gitStatusMetadataSubline(metadata)
			} else if path, ok := sessionMetadataPath(persisted.Sessions[name]); ok {
				item.Metadata = uity.SessionMetadataSubline{Kind: uity.MetadataKindDirectory, SessionName: name, Path: path}
			}
		}
		items = append(items, item)
		byName[name] = item
	}
	return items, byName
}

func coreLayoutFromPersisted(persisted *ports.SidebarLayout) sidebarlayout.Layout {
	if persisted == nil {
		return sidebarlayout.Layout{}
	}
	items := make([]sidebarlayout.LayoutItem, 0, len(persisted.Items))
	for _, item := range persisted.Items {
		switch item.Kind {
		case string(sidebarlayout.ItemKindCategory):
			if item.Category == nil {
				continue
			}
			sessions := make([]string, 0, len(item.Category.Sessions))
			for _, ref := range item.Category.Sessions {
				sessions = append(sessions, ref.Name)
			}
			items = append(items, sidebarlayout.CategoryItem(firstNonEmpty(item.Category.ID, item.ID), item.Category.Name, item.Category.Collapsed, sessions))
		case string(sidebarlayout.ItemKindSeparator):
			items = append(items, sidebarlayout.SeparatorItem(firstNonEmpty(item.ID, persistedSpacerID(item.Separator))))
		case string(sidebarlayout.ItemKindSpacer):
			items = append(items, sidebarlayout.SpacerItem(firstNonEmpty(item.ID, persistedSpacerID(item.Spacer))))
		}
	}
	return sidebarlayout.Layout{Items: items}
}

func persistedLayoutFromCore(layout sidebarlayout.Layout) *ports.SidebarLayout {
	items := make([]ports.SidebarLayoutItem, 0, len(layout.Items))
	for _, item := range layout.Items {
		switch item.Kind {
		case sidebarlayout.ItemKindCategory:
			refs := make([]ports.SidebarLayoutSessionRef, 0, len(item.Category.Sessions))
			for _, ref := range item.Category.Sessions {
				refs = append(refs, ports.SidebarLayoutSessionRef{Name: ref.Name})
			}
			items = append(items, ports.SidebarLayoutItem{ID: item.ID, Kind: string(item.Kind), Category: &ports.SidebarLayoutCategory{ID: item.Category.ID, Name: item.Category.Name, Collapsed: item.Category.Collapsed, Sessions: refs}})
		case sidebarlayout.ItemKindSeparator:
			items = append(items, ports.SidebarLayoutItem{ID: item.ID, Kind: string(item.Kind), Separator: &ports.SidebarLayoutSpacer{ID: item.ID}})
		case sidebarlayout.ItemKindSpacer:
			items = append(items, ports.SidebarLayoutItem{ID: item.ID, Kind: string(item.Kind), Spacer: &ports.SidebarLayoutSpacer{ID: item.ID}})
		}
	}
	return &ports.SidebarLayout{Items: items}
}

func saveReconciledSidebarLayout(ctx context.Context, live []string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		state.SidebarLayout = persistedLayoutFromCore(layout)
		state.SessionOrder = sessions.ApplyOrder(live, state.SessionOrder)
	})
}

func saveNewSidebarCategory(ctx context.Context, name string, live []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("create sidebar category: name is required")
	}
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		id := uniqueLayoutID("category", layout)
		layout.Items = append(layout.Items, sidebarlayout.CategoryItem(id, name, false, nil))
		state.SidebarLayout = persistedLayoutFromCore(layout)
	})
}

func saveSidebarSessionCategory(ctx context.Context, sessionName string, categoryID string, live []string) error {
	sessionName = strings.TrimSpace(sessionName)
	categoryID = strings.TrimSpace(categoryID)
	if sessionName == "" || categoryID == "" {
		return nil
	}
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		foundTarget := false
		for itemIndex := range layout.Items {
			if layout.Items[itemIndex].Kind != sidebarlayout.ItemKindCategory {
				continue
			}
			category := &layout.Items[itemIndex].Category
			kept := category.Sessions[:0]
			for _, ref := range category.Sessions {
				if ref.Name != sessionName {
					kept = append(kept, ref)
				}
			}
			category.Sessions = kept
			if category.ID == categoryID {
				foundTarget = true
				category.Sessions = append(category.Sessions, sidebarlayout.SessionRef{Name: sessionName})
			}
		}
		if foundTarget {
			state.SidebarLayout = persistedLayoutFromCore(layout)
		}
	})
}

func saveSidebarCategoryCollapsed(ctx context.Context, categoryID string, collapsed bool, live []string) error {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil
	}
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		for i := range layout.Items {
			if layout.Items[i].Kind == sidebarlayout.ItemKindCategory && layout.Items[i].Category.ID == categoryID {
				layout.Items[i].Category.Collapsed = collapsed
				break
			}
		}
		state.SidebarLayout = persistedLayoutFromCore(layout)
	})
}

func saveRenamedSidebarCategory(ctx context.Context, categoryID string, name string, live []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("rename sidebar category: name is required")
	}
	return withLoadedSidebarState(ctx, func(store storefs.Store, state *ports.PersistedState) error {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		found := false
		for i, item := range layout.Items {
			if item.Kind == sidebarlayout.ItemKindCategory && item.Category.ID == categoryID {
				layout.Items[i].Category.Name = name
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("rename sidebar category: category %q not found", categoryID)
		}
		state.SidebarLayout = persistedLayoutFromCore(layout)
		return saveLoadedSidebarState(ctx, store, *state)
	})
}

func saveNewSidebarSpacer(ctx context.Context, live []string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		layout.Items = append(layout.Items, sidebarlayout.SpacerItem(uniqueLayoutID("spacer", layout)))
		state.SidebarLayout = persistedLayoutFromCore(layout)
	})
}

func saveNewSidebarSeparator(ctx context.Context, live []string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		layout.Items = append(layout.Items, sidebarlayout.SeparatorItem(uniqueLayoutID("separator", layout)))
		state.SidebarLayout = persistedLayoutFromCore(layout)
	})
}

func currentLiveSessionNames(ctx context.Context) ([]string, error) {
	views, err := loadSessionViews(ctx)
	if err != nil {
		return nil, err
	}
	return sessionNames(sessions.FilterVisible(views, true)), nil
}

func sidebarlayoutSelectionForItem(itemID string) sidebarlayout.Selection {
	return sidebarlayout.SelectionForItemID(itemID)
}

func saveMovedSidebarLayoutItem(ctx context.Context, selection sidebarlayout.Selection, delta int, live []string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		layout = sidebarlayout.MoveSelectionVisible(layout, selection, delta, persistedShowNumeric(*state))
		state.SidebarLayout = persistedLayoutFromCore(layout)
	})
}

func saveDeletedSidebarLayoutItem(ctx context.Context, selection sidebarlayout.Selection, live []string) error {
	return updateSidebarState(ctx, func(state *ports.PersistedState) {
		layout := sidebarlayout.EnsureLayout(coreLayoutFromPersisted(state.SidebarLayout), live, state.SessionOrder)
		items := make([]sidebarlayout.LayoutItem, 0, len(layout.Items))
		for _, item := range layout.Items {
			if sidebarLayoutItemMatchesSelection(item, selection) {
				continue
			}
			items = append(items, item)
		}
		state.SidebarLayout = persistedLayoutFromCore(sidebarlayout.EnsureLayout(sidebarlayout.Layout{Items: items}, live, state.SessionOrder))
	})
}

func sidebarLayoutItemMatchesSelection(item sidebarlayout.LayoutItem, selection sidebarlayout.Selection) bool {
	switch selection.Kind {
	case sidebarlayout.RowKindCategory:
		return item.Kind == sidebarlayout.ItemKindCategory && item.Category.ID == selection.CategoryID
	case sidebarlayout.RowKindSeparator, sidebarlayout.RowKindSpacer:
		return item.ID == selection.ItemID
	default:
		return false
	}
}

func sidebarSelectionForCurrent(layout sidebarlayout.Layout, current string) sidebarlayout.Selection {
	var firstCategory sidebarlayout.Selection
	for _, item := range layout.Items {
		if item.Kind != sidebarlayout.ItemKindCategory {
			continue
		}
		category := item.Category
		if firstCategory.Kind == "" {
			firstCategory = sidebarlayout.Selection{Kind: sidebarlayout.RowKindCategory, CategoryID: category.ID, ItemID: item.ID}
		}
		if current == "" {
			continue
		}
		for _, ref := range category.Sessions {
			if ref.Name == current {
				return sidebarlayout.Selection{Kind: sidebarlayout.RowKindSession, Session: current, CategoryID: category.ID, ItemID: item.ID + "/session:" + current}
			}
		}
	}
	return firstCategory
}

func persistedShowNumeric(state ports.PersistedState) bool {
	return state.Sidebar != nil && state.Sidebar.ShowNumericSessions
}

func attentionStateMap(raw map[string][]byte) map[string]attention.State {
	return attention.DecodeStateMap(raw)
}

func heatDisplaysForNames(names []string, states map[string]heat.State, now time.Time, cfg ports.ConfigSnapshot) map[string]heat.Display {
	return heat.DisplayByRecentActivity(names, states, now, recentHeatWindow(cfg), cfg.HeatMaxHighlighted)
}

func persistedSpacerID(spacer *ports.SidebarLayoutSpacer) string {
	if spacer == nil {
		return ""
	}
	return spacer.ID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueLayoutID(prefix string, layout sidebarlayout.Layout) string {
	used := map[string]bool{}
	for _, item := range layout.Items {
		used[item.ID] = true
	}
	for i := 1; i <= 10000; i++ {
		id := fmt.Sprintf("%s:%d", prefix, i)
		if !used[id] {
			return id
		}
	}
	return fmt.Sprintf("%s:%d", prefix, time.Now().UnixNano())
}
