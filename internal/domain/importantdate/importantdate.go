// Package importantdate is the domain model for the "important dates"
// list: chronological, admin-curated bilingual milestones (registration
// opens, exam day, results) the client frontend renders. It defines only
// the entity and the persistence contract — no database or transport
// concerns.
package importantdate

import (
	"context"
	"time"
)

// ImportantDate is a single dated milestone. The title is English-only,
// but the details carry both an English and a Bengali string; the
// frontend picks one by the reader's locale, so both are always required.
type ImportantDate struct {
	ID string
	// EventDate is a calendar date (the DB column is DATE); the
	// time-of-day component is always zero.
	EventDate time.Time
	Title     string
	DetailsEN string
	DetailsBN string
	// DisplayOrder breaks ties between entries that fall on the same date.
	DisplayOrder int
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Repository abstracts persistence for ImportantDate so the application
// layer never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, d *ImportantDate) error
	// Update replaces the mutable fields of an existing row, returning
	// ErrNotFound when the id does not exist.
	Update(ctx context.Context, d *ImportantDate) error
	// Delete removes a row, returning ErrNotFound when the id does not
	// exist.
	Delete(ctx context.Context, id string) error
	// ListAll returns every row, active or not, ordered by event_date ASC
	// then display_order ASC. It backs the admin console.
	ListAll(ctx context.Context) ([]*ImportantDate, error)
	// ListActive returns only active rows, ordered by event_date ASC then
	// display_order ASC. It backs the public list.
	ListActive(ctx context.Context) ([]*ImportantDate, error)
}
