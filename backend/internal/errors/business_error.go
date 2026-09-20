package errors

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrInvalidInput = errors.New("invalid request input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrLockConflict = errors.New("an active price lock already exists for the product")
	ErrLockRejected = errors.New("price lock order rejected: one or more quotes are not lockable")
)

type BusinessError struct {
	Code    int
	Status  int
	Message string
	Err     error
}

func (e *BusinessError) Error() string { return e.Message }
func (e *BusinessError) Unwrap() error { return e.Err }
