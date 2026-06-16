package ports

import "time"

type ClockPort interface {
	Now() time.Time
}
