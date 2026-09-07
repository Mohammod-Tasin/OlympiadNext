// Package registration is the domain model for manual exam-registration
// payments: a student submits a bKash/Nagad transaction id after paying an
// event's fee, and an admin approves or rejects it against their payment
// statement. It defines only the entity and the persistence contract — no
// database or transport concerns.
package registration

import (
	"context"
	"time"
)

// PaymentMethod is the wallet the student paid from.
type PaymentMethod string

const (
	MethodBkash PaymentMethod = "bkash"
	MethodNagad PaymentMethod = "nagad"
)

// Valid reports whether m is one of the two supported wallets.
func (m PaymentMethod) Valid() bool {
	return m == MethodBkash || m == MethodNagad
}

// Status tracks where a registration sits in the manual review:
// 'pending' until an admin checks the payment, then 'approved' or
// 'rejected'. A rejected registration can be returned to pending by an
// admin correction, while the UNIQUE constraints still prevent a second
// submission for the same transaction or student/event.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Valid reports whether s is one of the three defined states.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected:
		return true
	default:
		return false
	}
}

// CanUnreject reports whether this status can enter the narrow admin
// correction path back to pending. It is intentionally not a general
// transition rule: only a rejected registration can be unrejected.
func (s Status) CanUnreject() bool {
	return s == StatusRejected
}

// Registration is a single student's payment submission for one event.
type Registration struct {
	ID            string
	UserID        string
	EventID       string
	PaymentMethod PaymentMethod
	SenderNumber  string
	TransactionID string
	Status        Status
	ReviewedBy    *string
	ReviewedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Detail is a Registration plus the joined display fields the review queue
// and the student's own list render. ListByUser leaves the student fields
// zero (the caller already knows who they are); ListByStatus fills all of
// them.
type Detail struct {
	Registration
	StudentName  *string
	StudentEmail string
	EventTitle   string
}

// Repository abstracts persistence for Registration so the application
// layer never depends on a concrete database driver.
type Repository interface {
	// Create inserts a new pending registration. It returns
	// ErrDuplicateTransactionID or ErrAlreadyRegistered when the
	// corresponding UNIQUE constraint is violated, so the caller can map
	// each to a distinct 409 rather than a generic 500.
	Create(ctx context.Context, r *Registration) error
	// FindByID returns ErrNotFound when the id does not exist.
	FindByID(ctx context.Context, id string) (*Registration, error)
	// ListByUser returns the caller's own registrations, newest first,
	// each with its EventTitle populated.
	ListByUser(ctx context.Context, userID string) ([]*Detail, error)
	// ListByStatus returns the review queue, newest first, each row joined
	// with the student's name/email and the event title. limit caps it.
	ListByStatus(ctx context.Context, status Status, limit int) ([]*Detail, error)
	// Review records an admin's decision: it sets status, reviewed_by and
	// reviewed_at in one UPDATE. status must be approved or rejected.
	// Returns ErrNotFound for an unknown id and ErrAlreadyReviewed when the
	// row is no longer pending.
	Review(ctx context.Context, id, reviewedBy string, status Status) error
	// Unreject returns a rejected registration to pending, clearing its
	// review metadata in the same guarded UPDATE. It returns
	// ErrInvalidUnrejectTransition unless the row is currently rejected.
	Unreject(ctx context.Context, id string) error
}
