// Package round is the domain model for an event's sequential rounds
// (Qualifying/Semifinal/Final by default, admin-renameable) and each
// student's per-round qualify/eliminate/winner decision. It defines only
// the entity and the persistence contract — no database or transport
// concerns.
package round

import (
	"context"
	"time"
)

// Status tracks where a round sits in its lifecycle: 'upcoming' until an
// admin opens it, 'ongoing' while students may enter, then 'ended' once
// closed for review. Transitions are forward-only (upcoming -> ongoing ->
// ended), enforced by Repository.SetStatus's guarded UPDATE.
type Status string

const (
	StatusUpcoming Status = "upcoming"
	StatusOngoing  Status = "ongoing"
	StatusEnded    Status = "ended"
)

// Valid reports whether s is one of the three defined states.
func (s Status) Valid() bool {
	switch s {
	case StatusUpcoming, StatusOngoing, StatusEnded:
		return true
	default:
		return false
	}
}

// ParticipantStatus is an admin's decision for one student in one round.
// 'winner' is only meaningful on an event's highest-numbered round.
type ParticipantStatus string

const (
	ParticipantQualified  ParticipantStatus = "qualified"
	ParticipantEliminated ParticipantStatus = "eliminated"
	ParticipantWinner     ParticipantStatus = "winner"
)

// Valid reports whether s is one of the three defined decisions.
func (s ParticipantStatus) Valid() bool {
	switch s {
	case ParticipantQualified, ParticipantEliminated, ParticipantWinner:
		return true
	default:
		return false
	}
}

// Round is a single numbered stage of an event.
type Round struct {
	ID              string
	EventID         string
	RoundOrder      int
	RoundName       string
	StartAt         time.Time
	DurationMinutes int
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Participant is one student's recorded decision for one round.
type Participant struct {
	ID        string
	RoundID   string
	UserID    string
	Status    ParticipantStatus
	DecidedAt time.Time
}

// ParticipantDetail is a Participant plus the joined student display
// fields the admin candidate list renders, mirroring
// registration.Detail's join-result shape.
type ParticipantDetail struct {
	Participant
	StudentName  *string
	StudentEmail string
}

// Repository abstracts persistence for Round/Participant so the
// application layer never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, r *Round) error
	// Update replaces a round's order/name/start/duration. It never
	// touches Status — that only moves via SetStatus.
	Update(ctx context.Context, r *Round) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*Round, error)
	// FindByEventAndOrder returns ErrNotFound when no round at that
	// position exists yet (e.g. round 1 has no "round 0").
	FindByEventAndOrder(ctx context.Context, eventID string, order int) (*Round, error)
	// ListByEvent returns every round for an event, ordered by round_order
	// ascending.
	ListByEvent(ctx context.Context, eventID string) ([]*Round, error)
	// MaxRoundOrder returns the highest round_order configured for an
	// event, or 0 if it has none yet.
	MaxRoundOrder(ctx context.Context, eventID string) (int, error)
	// SetStatus performs a guarded compare-and-swap (WHERE id = ? AND
	// status = from), returning ErrNotFound for an unknown id or
	// ErrInvalidTransition when the round is not currently in the from
	// state.
	SetStatus(ctx context.Context, id string, from, to Status) error

	// GetParticipant returns ErrParticipantNotFound when the student has
	// no decision recorded for that round.
	GetParticipant(ctx context.Context, roundID, userID string) (*Participant, error)
	// ListParticipantsByRound returns every decision recorded in one
	// round, used to overlay existing decisions onto a candidate list
	// without an N+1 lookup per candidate.
	ListParticipantsByRound(ctx context.Context, roundID string) ([]*Participant, error)
	// ListParticipantsWithUser returns the students in one round with a
	// given decision, joined with their name/email, sourcing the
	// candidate pool for the next round.
	ListParticipantsWithUser(ctx context.Context, roundID string, status ParticipantStatus) ([]*ParticipantDetail, error)
	// UpsertParticipants bulk-records decisions for a round, overwriting
	// any existing decision for the same (round, user).
	UpsertParticipants(ctx context.Context, roundID string, participants []Participant) error
}
