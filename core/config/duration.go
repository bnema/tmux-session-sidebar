package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseRelativeDuration parses compact user-facing intervals such as 10m, 2h, or 3d.
// Empty input disables the interval.
func ParseRelativeDuration(raw string) (time.Duration, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return 0, nil
	}
	unit := value[len(value)-1]
	number := strings.TrimSpace(value[:len(value)-1])
	amount, err := strconv.Atoi(number)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid relative duration %q", raw)
	}
	switch unit {
	case 'm':
		return time.Duration(amount) * time.Minute, nil
	case 'h':
		return time.Duration(amount) * time.Hour, nil
	case 'd':
		return time.Duration(amount) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid relative duration unit %q in %q", string(unit), raw)
	}
}
