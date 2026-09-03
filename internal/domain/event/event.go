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
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
}
