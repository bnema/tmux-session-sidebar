package app

import "github.com/bnema/tmux-session-sidebar/adapters/uity"

func toUITreeItems(items []sidebarTreeItem) []uity.TreeItem {
	uiItems := make([]uity.TreeItem, 0, len(items))
	for _, item := range items {
		uiItem := uity.TreeItem{
			ID:             item.ID,
			CategoryID:     item.CategoryID,
			CategoryName:   item.CategoryName,
			CategoryOpen:   item.CategoryOpen,
			Session:        item.Session,
			Slot:           item.Slot,
			Branch:         item.Branch,
			MetadataPrefix: item.MetadataPrefix,
			ShowMetadata:   item.ShowMetadata,
		}
		switch item.Kind {
		case sidebarTreeRowCategory:
			uiItem.Kind = uity.TreeRowCategory
		case sidebarTreeRowSession:
			uiItem.Kind = uity.TreeRowSession
		case sidebarTreeRowSeparator:
			uiItem.Kind = uity.TreeRowSeparator
		case sidebarTreeRowSpacer:
			uiItem.Kind = uity.TreeRowSpacer
		}
		uiItems = append(uiItems, uiItem)
	}
	return uiItems
}
