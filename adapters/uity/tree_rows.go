package uity

func SessionItemsToTree(items []SessionItem) []TreeItem {
	tree := make([]TreeItem, 0, len(items)+1)
	tree = append(tree, TreeItem{Kind: TreeRowCategory, ID: "category:default", CategoryID: "category:default", CategoryName: "Default", CategoryOpen: true})
	for i, item := range items {
		branch := "├─"
		metadataPrefix := "│  "
		if i == len(items)-1 {
			branch = "└─"
			metadataPrefix = "   "
		}
		tree = append(tree, TreeItem{
			Kind:           TreeRowSession,
			ID:             "category:default/session:" + item.Name,
			CategoryID:     "category:default",
			Session:        item,
			Slot:           item.Slot,
			Branch:         branch,
			MetadataPrefix: metadataPrefix,
			ShowMetadata:   true,
		})
	}
	return tree
}

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
