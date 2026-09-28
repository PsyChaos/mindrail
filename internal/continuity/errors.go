package continuity

import "errors"

var (
	ErrConflict        = errors.New("continuity revision conflict")
	ErrIntentNotFound  = errors.New("continuity intent not found")
	ErrTokenConsumed   = errors.New("continuity takeover token already consumed")
	ErrInvalidToken    = errors.New("continuity takeover token is invalid")
	ErrReservationLost = errors.New("continuity task reservation is no longer valid")
)
