package repository

import "errors"

// Sentinel errors let services distinguish a database-level conflict from
// ordinary persistence failures via errors.Is.
var (
	ErrLockDuplicate = errors.New("duplicate active price lock")
)
