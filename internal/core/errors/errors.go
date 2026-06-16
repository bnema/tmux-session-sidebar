package errors

import "errors"

var (
	ErrInvalidSessionName = errors.New("invalid session name")
	ErrMissingSession     = errors.New("missing session")
	ErrDuplicateSession   = errors.New("duplicate session")
	ErrLastSessionKill    = errors.New("cannot kill the last remaining session")
	ErrMissingClient      = errors.New("missing client")
	ErrNoQuickSwitchSlot  = errors.New("no quick-switch session for slot")
)
