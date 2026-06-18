package uity

func sessionItemsFromTree(treeItems []TreeItem) []SessionItem {
	items := make([]SessionItem, 0, len(treeItems))
	for _, item := range treeItems {
		if item.Kind != TreeRowSession {
			continue
		}
		session := item.Session
		if item.Slot > 0 {
			session.Slot = item.Slot
		}
		items = append(items, session)
	}
	return items
}
