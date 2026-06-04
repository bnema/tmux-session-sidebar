package sidebar

import (
	"reflect"
	"testing"
)

func TestEnsureLayoutMigratesFlatOrderIntoDefaultCategory(t *testing.T) {
	layout := EnsureLayout(Layout{}, []string{"alpha", "beta", "gamma"}, []string{"beta", "alpha"})

	if len(layout.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(layout.Items))
	}
	category := layout.Items[0].Category
	if category.ID != DefaultCategoryID || category.Name != DefaultCategoryName {
		t.Fatalf("default category = %#v, want default id/name", category)
	}
	if got, want := sessionNames(category.Sessions), []string{"beta", "alpha", "gamma"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions = %#v, want %#v", got, want)
	}
}

func TestEnsureLayoutReconcilesExistingLayout(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha", "missing"}),
		SeparatorItem("separator:one"),
		CategoryItem(DefaultCategoryID, DefaultCategoryName, false, []string{"beta", "alpha"}),
		SpacerItem("spacer:one"),
	}}

	got := EnsureLayout(layout, []string{"alpha", "beta", "gamma"}, []string{"beta", "alpha", "gamma"})

	if got.Items[1].Kind != ItemKindSeparator || got.Items[3].Kind != ItemKindSpacer {
		t.Fatalf("non-session layout items not preserved: %#v", got.Items)
	}
	if names := sessionNames(got.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha"}) {
		t.Fatalf("work sessions = %#v, want alpha", names)
	}
	if names := sessionNames(got.Items[2].Category.Sessions); !reflect.DeepEqual(names, []string{"beta", "gamma"}) {
		t.Fatalf("default sessions = %#v, want beta,gamma", names)
	}
}

func TestFlattenBuildsTreeRowsAndContextualSlots(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha", "beta", "3"}),
		SeparatorItem("separator:one"),
		CategoryItem("category:personal", "Personal", false, []string{"notes"}),
	}}
	selection := Selection{Kind: RowKindSession, CategoryID: "category:work", Session: "beta"}

	rows := Flatten(layout, selection, false)

	if len(rows) != 6 {
		t.Fatalf("rows len = %d, want 6: %#v", len(rows), rows)
	}
	assertRow(t, rows[0], TreeRow{Kind: RowKindCategory, ItemID: "category:work", CategoryID: "category:work", CategoryName: "Work", CategoryOpen: true})
	assertRow(t, rows[1], TreeRow{Kind: RowKindSession, ItemID: "category:work/session:alpha", CategoryID: "category:work", Session: "alpha", Slot: 1, Branch: "├─", MetadataPrefix: "│  "})
	assertRow(t, rows[2], TreeRow{Kind: RowKindSession, ItemID: "category:work/session:beta", CategoryID: "category:work", Session: "beta", Slot: 2, Branch: "└─", MetadataPrefix: "   "})
	assertRow(t, rows[3], TreeRow{Kind: RowKindSeparator, ItemID: "separator:one"})
	assertRow(t, rows[4], TreeRow{Kind: RowKindCategory, ItemID: "category:personal", CategoryID: "category:personal", CategoryName: "Personal", CategoryOpen: true})
	assertRow(t, rows[5], TreeRow{Kind: RowKindSession, ItemID: "category:personal/session:notes", CategoryID: "category:personal", Session: "notes", Branch: "└─", MetadataPrefix: "   "})
}

func TestActiveCategoryIDFallsBackToSessionLookup(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha"}),
		CategoryItem("category:personal", "Personal", false, []string{"notes"}),
	}}

	got := ActiveCategoryID(layout, Selection{Kind: RowKindSession, Session: "notes"})

	if got != "category:personal" {
		t.Fatalf("ActiveCategoryID() = %q, want category:personal", got)
	}
}

func TestFlattenSkipsCollapsedCategoryChildren(t *testing.T) {
	layout := Layout{Items: []LayoutItem{CategoryItem("category:work", "Work", true, []string{"alpha"})}}

	rows := Flatten(layout, Selection{Kind: RowKindCategory, CategoryID: "category:work"}, false)

	if len(rows) != 1 || rows[0].Kind != RowKindCategory || rows[0].CategoryOpen {
		t.Fatalf("rows = %#v, want one closed category row", rows)
	}
}

func TestMoveSelectionMovesTopLevelItems(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, nil),
		SeparatorItem("separator:one"),
		SpacerItem("spacer:one"),
	}}

	got := MoveSelection(layout, Selection{Kind: RowKindSeparator, ItemID: "separator:one"}, 1)

	if gotKinds := itemKinds(got.Items); !reflect.DeepEqual(gotKinds, []ItemKind{ItemKindCategory, ItemKindSpacer, ItemKindSeparator}) {
		t.Fatalf("item kinds = %#v, want category, spacer, separator", gotKinds)
	}
}

func TestMoveSelectionNormalizesLargeDeltas(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, nil),
		SeparatorItem("separator:one"),
		SpacerItem("spacer:one"),
	}}

	got := MoveSelection(layout, Selection{Kind: RowKindCategory, ItemID: "category:work"}, 3)

	if gotKinds := itemKinds(got.Items); !reflect.DeepEqual(gotKinds, []ItemKind{ItemKindSeparator, ItemKindCategory, ItemKindSpacer}) {
		t.Fatalf("item kinds = %#v, want one-step category move", gotKinds)
	}
}

func TestMoveSelectionMovesSessionAcrossCategories(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha", "beta"}),
		SeparatorItem("separator:one"),
		CategoryItem("category:personal", "Personal", false, []string{"notes"}),
	}}

	movedDown := MoveSelection(layout, Selection{Kind: RowKindSession, CategoryID: "category:work", Session: "beta"}, 1)
	if names := sessionNames(movedDown.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha"}) {
		t.Fatalf("work after move down = %#v, want alpha", names)
	}
	if names := sessionNames(movedDown.Items[2].Category.Sessions); !reflect.DeepEqual(names, []string{"beta", "notes"}) {
		t.Fatalf("personal after move down = %#v, want beta,notes", names)
	}

	movedUp := MoveSelection(movedDown, Selection{Kind: RowKindSession, CategoryID: "category:personal", Session: "beta"}, -1)
	if names := sessionNames(movedUp.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
		t.Fatalf("work after move up = %#v, want alpha,beta", names)
	}
	if names := sessionNames(movedUp.Items[2].Category.Sessions); !reflect.DeepEqual(names, []string{"notes"}) {
		t.Fatalf("personal after move up = %#v, want notes", names)
	}
}

func TestMoveSelectionResolvesSessionCategoryWhenSelectionOmitsIt(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha"}),
		CategoryItem("category:personal", "Personal", false, []string{"notes"}),
	}}

	got := MoveSelection(layout, Selection{Kind: RowKindSession, Session: "notes"}, -1)

	if names := sessionNames(got.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha", "notes"}) {
		t.Fatalf("work after session-only move = %#v, want alpha,notes", names)
	}
	if names := sessionNames(got.Items[1].Category.Sessions); len(names) != 0 {
		t.Fatalf("personal after session-only move = %#v, want empty", names)
	}
}

func TestMoveSelectionNormalizesStaleEmbeddedCategoryID(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		{ID: "category:work", Kind: ItemKindCategory, Category: Category{Name: "Work", Sessions: []SessionRef{{Name: "alpha"}}}},
		CategoryItem("category:personal", "Personal", false, []string{"notes"}),
	}}

	got := MoveSelection(layout, Selection{Kind: RowKindSession, CategoryID: "category:work", Session: "alpha"}, 1)

	if names := sessionNames(got.Items[0].Category.Sessions); len(names) != 0 {
		t.Fatalf("work after stale-id move = %#v, want empty", names)
	}
	if names := sessionNames(got.Items[1].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha", "notes"}) {
		t.Fatalf("personal after stale-id move = %#v, want alpha,notes", names)
	}
}

func sessionNames(refs []SessionRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return names
}

func itemKinds(items []LayoutItem) []ItemKind {
	kinds := make([]ItemKind, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}

func assertRow(t *testing.T, got TreeRow, want TreeRow) {
	t.Helper()
	if got != want {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}
