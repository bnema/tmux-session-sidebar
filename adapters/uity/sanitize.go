package uity

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func sanitizeSessionName(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == '\x1b' {
			i += size
			if i < len(name) && name[i] == '[' {
				i++
				for i < len(name) {
					c := name[i]
					i++
					if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
						break
					}
				}
			}
			continue
		}
		if !unicode.IsControl(r) {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}
