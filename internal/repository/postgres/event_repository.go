package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/event"
)

const eventColumns = `id, title, description, image_url, event_date, is_active, created_at, updated_at`

type EventRepository struct {
	db *sql.DB
}

func NewEventRepository(db *sql.DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(ctx context.Context, e *event.Event) error {
	const q = `
		INSERT INTO events (id, title, description, image_url, event_date, is_active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, e.Title, e.Description, e.ImageURL, e.EventDate, e.IsActive).
		Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("event_repository: create failed: %w", err)
	}
	return nil
}

func (r *EventRepository) Update(ctx context.Context, e *event.Event) error {
	const q = `
		UPDATE events
		SET title = $1, description = $2, image_url = $3, event_date = $4, is_active = $5, updated_at = now()
		WHERE id = $6
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, q, e.Title, e.Description, e.ImageURL, e.EventDate, e.IsActive, e.ID).
		Scan(&e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return event.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("event_repository: update failed: %w", err)
	}
	return nil
}

func (r *EventRepository) FindByID(ctx context.Context, id string) (*event.Event, error) {
	const q = `SELECT ` + eventColumns + ` FROM events WHERE id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *EventRepository) FindActive(ctx context.Context) (*event.Event, error) {
	const q = `
		SELECT ` + eventColumns + `
		FROM events
		WHERE is_active = true
		ORDER BY event_date DESC, updated_at DESC
		LIMIT 1`
	return r.scanOne(r.db.QueryRowContext(ctx, q))
}

func (r *EventRepository) scanOne(row *sql.Row) (*event.Event, error) {
	var e event.Event
	err := row.Scan(&e.ID, &e.Title, &e.Description, &e.ImageURL, &e.EventDate, &e.IsActive, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, event.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("event_repository: scan failed: %w", err)
	}
	return &e, nil
}
