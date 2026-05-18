package sessions

import "testing"

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "simple", in: "alpha", wantErr: false},
		{name: "mixed allowed", in: "A_1-z", wantErr: false},
		{name: "numeric allowed", in: "123", wantErr: false},
		{name: "empty", in: "", wantErr: true},
		{name: "space", in: "bad name", wantErr: true},
		{name: "slash", in: "bad/name", wantErr: true},
		{name: "colon", in: "bad:name", wantErr: true},
		{name: "dot", in: "bad.name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.in)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("ValidateName(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestIsNumericName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "numeric", in: "123", want: true},
		{name: "zero", in: "0", want: true},
		{name: "mixed", in: "123a", want: false},
		{name: "empty", in: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNumericName(tt.in); got != tt.want {
				t.Fatalf("IsNumericName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFilterVisible(t *testing.T) {
	all := []View{
		{SessionID: "1", Name: "alpha", Visible: true},
		{SessionID: "2", Name: "123", Visible: true},
		{SessionID: "3", Name: "hidden", Visible: false},
	}
	tests := []struct {
		name        string
		showNumeric bool
		wantNames   []string
	}{
		{name: "hide numeric", showNumeric: false, wantNames: []string{"alpha"}},
		{name: "show numeric", showNumeric: true, wantNames: []string{"alpha", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterVisible(all, tt.showNumeric)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.wantNames), got)
			}
			for i, want := range tt.wantNames {
				if got[i].Name != want {
					t.Fatalf("name[%d] = %q, want %q", i, got[i].Name, want)
				}
			}
		})
	}
}
