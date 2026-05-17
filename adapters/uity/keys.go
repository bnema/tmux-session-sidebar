package uity

type Intent string

const (
	IntentNone             Intent = "none"
	IntentClose            Intent = "close"
	IntentSwitch           Intent = "switch"
	IntentEnterSearch      Intent = "enter-search"
	IntentApplySearch      Intent = "apply-search"
	IntentCancelSearch     Intent = "cancel-search"
	IntentMoveDown         Intent = "move-down"
	IntentMoveUp           Intent = "move-up"
	IntentCreateProject    Intent = "create-project"
	IntentCreateGitProject Intent = "create-git-project"
	IntentCreateAdhoc      Intent = "create-adhoc"
	IntentRename           Intent = "rename"
	IntentKill             Intent = "kill"
	IntentToggleNumeric    Intent = "toggle-numeric"
)

type Mode string

const (
	ModeBrowse  Mode = "browse"
	ModeSearch  Mode = "search"
	ModeProject Mode = "project"
)

func InterpretKey(mode Mode, seq []byte) Intent {
	if len(seq) == 0 {
		return IntentNone
	}
	if seq[0] == 0x1b {
		if len(seq) == 1 {
			if mode == ModeSearch {
				return IntentCancelSearch
			}
			return IntentClose
		}
		switch seq[1] {
		case 'n':
			return IntentCreateProject
		case 'g':
			return IntentCreateGitProject
		case 'a':
			return IntentCreateAdhoc
		case 'r':
			return IntentRename
		case 'x':
			return IntentKill
		case 'h':
			return IntentToggleNumeric
		}
		if mode == ModeSearch {
			return IntentCancelSearch
		}
		return IntentClose
	}
	if seq[0] == '\r' || seq[0] == '\n' {
		if mode == ModeSearch {
			return IntentApplySearch
		}
		return IntentSwitch
	}
	if mode == ModeBrowse {
		switch seq[0] {
		case 'j':
			return IntentMoveDown
		case 'k':
			return IntentMoveUp
		case '/':
			return IntentEnterSearch
		}
	}
	return IntentNone
}
