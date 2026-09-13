// Package prize is the domain model for an event's admin-configured prize
// tiers: named rewards for a range of final-round placements (e.g. "1st
// place" or "2nd-3rd place"), informational content the client frontend
// renders. It defines only the entity and the persistence contract — no
// database or transport concerns.
package prize

import (
	"context"
	"time"
)

// Prize is one named reward tier for an inclusive range of final-round
// ranks. PrizeDescription is optional (the DB column is nullable). Level
// scopes the tier to one of the three academic levels ("Junior",
// "Secondary", "Higher Secondary") — two levels may configure identical
// or overlapping rank ranges without conflict.
type Prize struct {
	ID               string
	EventID          string
	Level            string
	RankFrom         int
	RankTo           int
	PrizeName        string
	PrizeDescription *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Repository abstracts persistence for Prize so the application layer
// never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, p *Prize) error
	// Update replaces the mutable fields of an existing prize tier,
	// returning ErrNotFound when the id does not exist.
	Update(ctx context.Context, p *Prize) error
	// Delete removes a prize tier, returning ErrNotFound when the id does
	// not exist.
	Delete(ctx context.Context, id string) error
	// ListByEvent returns every prize tier for an event, ordered by
	// rank_from ascending. Backs both the admin and public listings —
	// a prize tier has no draft/active state to distinguish them.
	ListByEvent(ctx context.Context, eventID string) ([]*Prize, error)
}
