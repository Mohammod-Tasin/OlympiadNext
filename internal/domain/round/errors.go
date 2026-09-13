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
	// ErrDuplicateFinal is the DB-constraint fallback for
	// uq_rounds_one_final_per_event — the service layer already checks
	// this and returns a friendly rounds.ErrValidation first, so this only
	// surfaces on a genuine concurrent-request race.
	ErrDuplicateFinal = errors.New("only one final round is allowed per event")
	// ErrDuplicateRank is the DB-constraint fallback for
	// uq_round_participants_rank, for the same reason: rounds.Service
	// already rejects a duplicate rank within one request, so this only
	// surfaces on a race between two separate requests.
	ErrDuplicateRank = errors.New("rank is already assigned to another participant in this round")
)
