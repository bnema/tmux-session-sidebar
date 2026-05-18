package uity

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bnema/tmux-session-sidebar/core/heat"
	"github.com/bnema/tmux-session-sidebar/core/quickswitch"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

type Capability string

const (
	CapabilityRGB   Capability = "rgb"
	Capability256   Capability = "256"
	CapabilityBasic Capability = "basic"
	CapabilityPlain Capability = "plain"
)

type Row struct {
	Session sessions.View
	Bucket  heat.Bucket
	Slot    int
}

type Screen struct {
	Rows                []Row
	Capability          Capability
	Mode                Mode
	Filter              string
	ShowNumericSessions bool
}

func RenderScreen(screen Screen) string {
	var b strings.Builder
	fmt.Fprintf(&b, "sessions %s", screen.Mode)
	if screen.Filter != "" {
		fmt.Fprintf(&b, " filter:%s", sanitizeSessionName(screen.Filter))
	}
	fmt.Fprintf(&b, " nums:%s\n", numberedStatus(screen.ShowNumericSessions))
	fmt.Fprintf(&b, "↵ switch  / search  Esc close  M-n project\n")
	fmt.Fprintf(&b, "M-g git  M-a adhoc  M-r rename  M-x kill  M-h nums\n\n")
	b.WriteString(Render(screen.Rows, screen.Capability))
	return b.String()
}

func Render(rows []Row, capability Capability) string {
	var b strings.Builder
	for _, row := range rows {
		style := HeatStyle(row.Bucket, capability)
		reset := ""
		if style != "" {
			reset = "\033[0m"
		}
		marker := " "
		if row.Session.Current {
			marker = "*"
		}
		badge := quickswitch.BadgeForSlot(row.Slot)
		if badge != "" {
			badge += " "
		}
		fmt.Fprintf(&b, "%s%s %s%s%s\n", style, marker, badge, sanitizeSessionName(row.Session.Name), reset)
	}
	return b.String()
}

func numberedStatus(show bool) string {
	if show {
		return "on"
	}
	return "off"
}

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

func HeatStyle(bucket heat.Bucket, capability Capability) string {
	switch capability {
	case CapabilityRGB:
		switch bucket {
		case heat.BucketCurrent:
			return "\033[1;38;2;152;251;152m"
		case heat.BucketHot:
			return "\033[38;2;122;232;122m"
		case heat.BucketWarm:
			return "\033[38;2;106;198;106m"
		case heat.BucketCool:
			return "\033[38;2;124;154;124m"
		case heat.BucketStale:
			return "\033[2;38;2;140;140;140m"
		}
	case Capability256:
		switch bucket {
		case heat.BucketCurrent:
			return "\033[1;38;5;121m"
		case heat.BucketHot:
			return "\033[38;5;114m"
		case heat.BucketWarm:
			return "\033[38;5;108m"
		case heat.BucketCool:
			return "\033[38;5;72m"
		case heat.BucketStale:
			return "\033[2;38;5;244m"
		}
	case CapabilityBasic:
		if bucket == heat.BucketCurrent || bucket == heat.BucketHot {
			return "\033[32m"
		}
		if bucket == heat.BucketStale {
			return "\033[2m"
		}
	}
	return ""
}
