package sidebar

import (
	"reflect"
	"testing"
)

func TestAddCategoryUseCaseEnsuresLayoutAndAllocatesID(t *testing.T) {
	got, err := AddCategory(Layout{}, []string{"alpha"}, nil, " Work ")
	if err != nil {
		t.Fatalf("AddCategory() error = %v", err)
	}

	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want default category plus new category: %#v", len(got.Items), got.Items)
	}
	if got.Items[1].ID != "category:1" || got.Items[1].Category.Name != "Work" {
		t.Fatalf("added category = %#v, want allocated category:1 named Work", got.Items[1])
	}
	if names := sessionNames(got.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha"}) {
		t.Fatalf("default sessions = %#v, want alpha", names)
	}
}

func TestAssignSessionCategoryUseCaseMovesSessionOnlyWhenTargetExists(t *testing.T) {
	layout := Layout{Items: []LayoutItem{
		CategoryItem("category:work", "Work", false, []string{"alpha", "beta"}),
		CategoryItem("category:personal", "Personal", false, nil),
	}}

	got, changed := AssignSessionCategory(layout, []string{"alpha", "beta"}, nil, " beta ", " category:personal ")
	if !changed {
		t.Fatalf("AssignSessionCategory() changed = false, want true")
	}
	if names := sessionNames(got.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha"}) {
		t.Fatalf("work sessions = %#v, want alpha", names)
	}
	if names := sessionNames(got.Items[1].Category.Sessions); !reflect.DeepEqual(names, []string{"beta"}) {
		t.Fatalf("personal sessions = %#v, want beta", names)
	}

	unchanged, changed := AssignSessionCategory(got, []string{"alpha", "beta"}, nil, "beta", "category:personal")
	if changed {
		t.Fatalf("already assigned target changed = true, want false")
	}
	if names := sessionNames(unchanged.Items[1].Category.Sessions); !reflect.DeepEqual(names, []string{"beta"}) {
		t.Fatalf("already assigned personal sessions = %#v, want beta", names)
	}

	unchanged, changed = AssignSessionCategory(layout, []string{"alpha", "beta"}, nil, "beta", "category:missing")
	if changed {
		t.Fatalf("missing target changed = true, want false")
	}
	if names := sessionNames(unchanged.Items[0].Category.Sessions); !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
		t.Fatalf("missing target work sessions = %#v, want alpha,beta", names)
	}
	if names := sessionNames(unchanged.Items[1].Category.Sessions); len(names) != 0 {
		t.Fatalf("missing target personal sessions = %#v, want empty", names)
	}
}

func TestDecorativeItemAndDeleteUseCasesOwnLayoutTransforms(t *testing.T) {
	layout := Layout{Items: []LayoutItem{CategoryItem(DefaultCategoryID, DefaultCategoryName, false, []string{"alpha"})}}

	withSpacer := AddSpacer(layout, []string{"alpha"}, nil)
	withSeparator := AddSeparator(withSpacer, []string{"alpha"}, nil)
	if gotKinds := itemKinds(withSeparator.Items); !reflect.DeepEqual(gotKinds, []ItemKind{ItemKindCategory, ItemKindSpacer, ItemKindSeparator}) {
		t.Fatalf("item kinds = %#v, want category, spacer, separator", gotKinds)
	}
	if withSpacer.Items[1].ID != "spacer:1" || withSeparator.Items[2].ID != "separator:1" {
		t.Fatalf("allocated IDs = %q/%q, want spacer:1/separator:1", withSpacer.Items[1].ID, withSeparator.Items[2].ID)
	}

	deleted := DeleteSelection(withSeparator, []string{"alpha"}, nil, Selection{Kind: RowKindSpacer, ItemID: "spacer:1"})
	if gotKinds := itemKinds(deleted.Items); !reflect.DeepEqual(gotKinds, []ItemKind{ItemKindCategory, ItemKindSeparator}) {
		t.Fatalf("after delete item kinds = %#v, want category, separator", gotKinds)
	}
}

func TestCategoryMetadataUseCasesValidateNotFoundLikeAppBoundary(t *testing.T) {
	layout := Layout{Items: []LayoutItem{CategoryItem("category:work", "Work", false, nil)}}

	colored, changed, err := SetCategoryColor(layout, []string{}, nil, " category:work ", " blue ")
	if err != nil || !changed {
		t.Fatalf("SetCategoryColor() error/changed = %v/%v, want nil/true", err, changed)
	}
	if colored.Items[0].Category.Color != "blue" {
		t.Fatalf("color = %q, want blue", colored.Items[0].Category.Color)
	}
	if _, changed, err := SetCategoryColor(colored, []string{}, nil, "category:work", "blue"); err != nil || changed {
		t.Fatalf("SetCategoryColor() no-op error/changed = %v/%v, want nil/false", err, changed)
	}
	if _, _, err := SetCategoryColor(layout, []string{}, nil, "category:missing", "blue"); err == nil {
		t.Fatalf("SetCategoryColor() missing non-empty color error = nil, want error")
	}
	if _, changed, err := SetCategoryColor(layout, []string{}, nil, "category:missing", ""); err != nil || changed {
		t.Fatalf("SetCategoryColor() missing empty color error/changed = %v/%v, want nil/false", err, changed)
	}

	renamed, err := RenameCategory(layout, []string{}, nil, "category:work", " Focus ")
	if err != nil {
		t.Fatalf("RenameCategory() error = %v", err)
	}
	if renamed.Items[0].Category.Name != "Focus" {
		t.Fatalf("name = %q, want Focus", renamed.Items[0].Category.Name)
	}
	if _, err := RenameCategory(layout, []string{}, nil, "category:missing", "Focus"); err == nil {
		t.Fatalf("RenameCategory() missing error = nil, want error")
	}

	collapsed, changed := SetCategoryCollapsed(layout, []string{}, nil, "category:work", true)
	if !changed {
		t.Fatalf("SetCategoryCollapsed() changed = false, want true")
	}
	unchanged, changed := SetCategoryCollapsed(collapsed, []string{}, nil, "category:work", true)
	if changed || !unchanged.Items[0].Category.Collapsed {
		t.Fatalf("SetCategoryCollapsed() no-op changed/collapsed = %v/%v, want false/true", changed, unchanged.Items[0].Category.Collapsed)
	}
	expanded, changed := SetCategorySessionsExpanded(collapsed, []string{}, nil, "category:work", true)
	if !changed {
		t.Fatalf("SetCategorySessionsExpanded() changed = false, want true")
	}
	unchanged, changed = SetCategorySessionsExpanded(expanded, []string{}, nil, "category:work", true)
	if changed || !unchanged.Items[0].Category.SessionsExpanded {
		t.Fatalf("SetCategorySessionsExpanded() no-op changed/expanded = %v/%v, want false/true", changed, unchanged.Items[0].Category.SessionsExpanded)
	}
	if !expanded.Items[0].Category.Collapsed || !expanded.Items[0].Category.SessionsExpanded {
		t.Fatalf("flags = collapsed:%v expanded:%v, want true/true", expanded.Items[0].Category.Collapsed, expanded.Items[0].Category.SessionsExpanded)
	}
}
