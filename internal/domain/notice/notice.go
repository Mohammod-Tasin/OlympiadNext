// Package notice is the domain model for the notice board: short,
// admin-curated bilingual announcements the client frontend renders as a
// scrolling list. It defines only the entity and the persistence
// contract — no database or transport concerns.
package notice

import (
	"context"
	"time"
)

// Notice is a single announcement. Every notice carries both an English
// and a Bengali string; the frontend picks one by the reader's locale,
// so both are always required.
type Notice struct {
	ID     string
	TextEN string
	TextBN string
	// DisplayOrder sorts the board ascending; ties fall back to created_at.
	DisplayOrder int
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Repository abstracts persistence for Notice so the application layer
// never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, n *Notice) error
	// Update replaces the mutable fields of an existing notice, returning
	// ErrNotFound when the id does not exist.
	Update(ctx context.Context, n *Notice) error
	// Delete removes a notice, returning ErrNotFound when the id does not
	// exist.
	Delete(ctx context.Context, id string) error
	// ListAll returns every notice, active or not, ordered by
	// display_order ASC then created_at ASC. It backs the admin console.
	ListAll(ctx context.Context) ([]*Notice, error)
	// ListActive returns only active notices, ordered by display_order
	// ASC then created_at ASC. It backs the public board.
	ListActive(ctx context.Context) ([]*Notice, error)
}
