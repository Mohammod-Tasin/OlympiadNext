package round

import "errors"

var (
	ErrNotFound            = errors.New("round not found")
	ErrParticipantNotFound = errors.New("round participant not found")
	// ErrDuplicateOrder is returned when a round_order is reused within
	// the same event (the uq_rounds_event_order constraint).
	ErrDuplicateOrder = errors.New("a round with this round_order already exists for this event")
	// ErrInvalidTransition is returned when SetStatus is called on a round
	// that is not currently in the expected "from" state.
	ErrInvalidTransition = errors.New("invalid round status transition")
	// ErrNotEnded is returned by the candidates/participants operations
	// when the round has not been ended yet.
	ErrNotEnded = errors.New("round has not ended yet")
)
