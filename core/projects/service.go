package projects

import (
	"path/filepath"
	"strings"
	"unicode"
)

type Root struct {
	Path string
}

type Candidate struct {
	Path        string
	SessionName string
}

func CandidateFromPath(path string) Candidate {
	return Candidate{Path: path, SessionName: NormalizeSessionName(filepath.Base(path))}
}

func NormalizeSessionName(base string) string {
	lower := strings.ToLower(base)
	var b strings.Builder
	lastUnderscore := false
	lastHyphen := false

	for _, r := range lower {
		switch {
		case (r >= 'a' && r <= 'z') || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
			lastHyphen = false
		case r == '_':
			if !lastUnderscore {
				b.WriteRune('_')
			}
			lastUnderscore = true
			lastHyphen = false
		case r == '-':
			if !lastHyphen {
				b.WriteRune('-')
			}
			lastHyphen = true
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteRune('_')
			}
			lastUnderscore = true
			lastHyphen = false
		}
	}

	name := strings.Trim(b.String(), "._-")
	if name == "" {
		return "session"
	}
	return name
}
