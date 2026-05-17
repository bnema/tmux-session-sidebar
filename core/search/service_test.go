package search

import "testing"

func TestMatchRanksSessions(t *testing.T) {
	items := []Item{
		{ID: "zeta", Label: "zeta"},
		{ID: "alpha", Label: "alpha"},
		{ID: "beta", Label: "beta"},
		{ID: "alphabet", Label: "alphabet"},
		{ID: "aleph", Label: "aleph"},
		{ID: "cafe", Label: "café"},
		{ID: "nihon", Label: "日本語"},
	}

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{name: "exact before prefix", query: "alpha", wantIDs: []string{"alpha", "alphabet"}},
		{name: "prefix before subsequence", query: "alp", wantIDs: []string{"alpha", "alphabet", "aleph"}},
		{name: "substring", query: "eta", wantIDs: []string{"beta", "zeta"}},
		{name: "subsequence", query: "zp", wantIDs: []string{}},
		{name: "case insensitive", query: "ALP", wantIDs: []string{"alpha", "alphabet", "aleph"}},
		{name: "accent prefix", query: "café", wantIDs: []string{"cafe"}},
		{name: "japanese prefix", query: "日本", wantIDs: []string{"nihon"}},
		{name: "empty returns all", query: "", wantIDs: []string{"aleph", "alpha", "alphabet", "beta", "cafe", "zeta", "nihon"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Match(items, tt.query)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.wantIDs), got)
			}
			for i, want := range tt.wantIDs {
				if got[i].Item.ID != want {
					t.Fatalf("id[%d] = %q, want %q (results=%v)", i, got[i].Item.ID, want, got)
				}
			}
		})
	}
}
