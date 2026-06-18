package uity

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func sanitizeSessionName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == '\x1b' {
			i += size
			if i >= len(name) {
				continue
			}
			switch name[i] {
			case '[':
				i++
				for i < len(name) {
					c := name[i]
					i++
					if c >= '@' && c <= '~' {
						break
					}
				}
			case ']':
				i++
				for i < len(name) {
					if name[i] == '\x07' {
						i++
						break
					}
					if name[i] == '\x1b' && i+1 < len(name) && name[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			default:
				i++
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
