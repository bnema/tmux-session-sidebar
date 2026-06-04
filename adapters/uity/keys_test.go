package uity

import "testing"

func TestInterpretKey(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		seq  []byte
		want Intent
	}{
		{name: "browse enter switches", mode: ModeBrowse, seq: []byte("\r"), want: IntentSwitch},
		{name: "search enter applies", mode: ModeSearch, seq: []byte("\r"), want: IntentApplySearch},
		{name: "browse slash enters search", mode: ModeBrowse, seq: []byte("/"), want: IntentEnterSearch},
		{name: "browse escape closes", mode: ModeBrowse, seq: []byte{0x1b}, want: IntentClose},
		{name: "search escape cancels", mode: ModeSearch, seq: []byte{0x1b}, want: IntentCancelSearch},
		{name: "alt n project", mode: ModeBrowse, seq: []byte{0x1b, 'n'}, want: IntentCreateProject},
		{name: "alt g git", mode: ModeBrowse, seq: []byte{0x1b, 'g'}, want: IntentCreateGitProject},
		{name: "alt a adhoc", mode: ModeBrowse, seq: []byte{0x1b, 'a'}, want: IntentCreateAdhoc},
		{name: "alt r rename", mode: ModeBrowse, seq: []byte{0x1b, 'r'}, want: IntentRename},
		{name: "alt x kill", mode: ModeBrowse, seq: []byte{0x1b, 'x'}, want: IntentKill},
		{name: "alt h numbers", mode: ModeBrowse, seq: []byte{0x1b, 'h'}, want: IntentToggleNumeric},
		{name: "alt H numbers", mode: ModeBrowse, seq: []byte{0x1b, 'H'}, want: IntentToggleNumeric},
		{name: "alt question mark help", mode: ModeBrowse, seq: []byte{0x1b, '?'}, want: IntentToggleHelp},
		{name: "plain n project", mode: ModeBrowse, seq: []byte("n"), want: IntentCreateProject},
		{name: "plain g git", mode: ModeBrowse, seq: []byte("g"), want: IntentCreateGitProject},
		{name: "plain a adhoc", mode: ModeBrowse, seq: []byte("a"), want: IntentCreateAdhoc},
		{name: "plain r rename", mode: ModeBrowse, seq: []byte("r"), want: IntentRename},
		{name: "plain x kill", mode: ModeBrowse, seq: []byte("x"), want: IntentKill},
		{name: "plain h numbers", mode: ModeBrowse, seq: []byte("h"), want: IntentToggleNumeric},
		{name: "plain question mark help", mode: ModeBrowse, seq: []byte("?"), want: IntentToggleHelp},
		{name: "plain shortcuts ignored in search", mode: ModeSearch, seq: []byte("n"), want: IntentNone},
		{name: "j moves down", mode: ModeBrowse, seq: []byte("j"), want: IntentMoveDown},
		{name: "k moves up", mode: ModeBrowse, seq: []byte("k"), want: IntentMoveUp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InterpretKey(tt.mode, tt.seq); got != tt.want {
				t.Fatalf("InterpretKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
