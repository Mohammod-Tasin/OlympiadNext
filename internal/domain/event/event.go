// Package event is the domain model for platform events: the dynamic,
// admin-curated content blocks the client frontend renders (upcoming
// olympiads, announcements, banners). It defines only the entity and the
// persistence contract — no database or transport concerns.
package event

import (
	"context"
	"time"
)

// Event is a single piece of admin-managed content. Exactly one event is
// expected to be active at a time; the client surface renders that one.
type Event struct {
	ID          string
	Title       string
	Description string
	ImageURL    string
	EventDate   time.Time
	IsActive    bool
	// Per-event manual-payment details for exam registration. BkashNumber
	// and NagadNumber are the merchant numbers a student sends the fee to;
	// RegistrationFee is whole Bangladeshi Taka (0 = not set yet). They are
	// surfaced on the public client event so the payment page can render
	// them.
	BkashNumber     string
	NagadNumber     string
	RegistrationFee int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Repository abstracts persistence for Event so the application layer
// never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, e *Event) error
	Update(ctx context.Context, e *Event) error
	FindByID(ctx context.Context, id string) (*Event, error)
	// FindActive returns the most recent active event, or ErrNotFound
	// when nothing is currently published.
	FindActive(ctx context.Context) (*Event, error)
	// ListAll returns every event, active or not, ordered with active
	// events first and then by event_date descending within each group —
	// see the postgres implementation's doc comment for why. Backs the
	// public multi-event listing (GET /api/client/events/all), a superset
	// of what FindActive alone can show.
	ListAll(ctx context.Context) ([]*Event, error)
}
