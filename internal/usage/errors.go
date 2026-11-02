package usage

import "errors"

var (
	ErrInvalid         = errors.New("invalid argument")
	ErrRangeInvalid    = errors.New("range invalid")
	ErrCSVZone         = errors.New("csv timezone required")
	ErrNotFound        = errors.New("not found")
	ErrClassInvalid    = errors.New("class invalid")
	ErrResetIncomplete = errors.New("reset incomplete")
	ErrLimitNear       = errors.New("limit near")
)
