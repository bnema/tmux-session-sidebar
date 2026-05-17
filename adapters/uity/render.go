package uity

import (
	"fmt"
	"strings"

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
			marker = "●"
		}
		badge := quickswitch.BadgeForSlot(row.Slot)
		if badge != "" {
			badge += " "
		}
		fmt.Fprintf(&b, "%s%s %s%s%s\n", style, marker, badge, row.Session.Name, reset)
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
