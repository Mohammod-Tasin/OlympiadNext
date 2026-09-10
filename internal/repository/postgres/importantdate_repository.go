package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/importantdate"
)

const importantDateColumns = `id, event_date, title, details_en, details_bn, display_order, is_active, created_at, updated_at`

type ImportantDateRepository struct {
	db *sql.DB
}

func NewImportantDateRepository(db *sql.DB) *ImportantDateRepository {
	return &ImportantDateRepository{db: db}
}

func (r *ImportantDateRepository) Create(ctx context.Context, d *importantdate.ImportantDate) error {
	const q = `
		INSERT INTO important_dates (id, event_date, title, details_en, details_bn, display_order, is_active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, now(), now())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, d.EventDate, d.Title, d.DetailsEN, d.DetailsBN, d.DisplayOrder, d.IsActive).
		Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("importantdate_repository: create failed: %w", err)
	}
	return nil
}

func (r *ImportantDateRepository) Update(ctx context.Context, d *importantdate.ImportantDate) error {
	const q = `
		UPDATE important_dates
		SET event_date = $1, title = $2, details_en = $3, details_bn = $4, display_order = $5, is_active = $6, updated_at = now()
		WHERE id = $7
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, q, d.EventDate, d.Title, d.DetailsEN, d.DetailsBN, d.DisplayOrder, d.IsActive, d.ID).
		Scan(&d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return importantdate.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("importantdate_repository: update failed: %w", err)
	}
	return nil
}

func (r *ImportantDateRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM important_dates WHERE id = $1`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("importantdate_repository: delete failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("importantdate_repository: delete rows affected failed: %w", err)
	}
	if n == 0 {
		return importantdate.ErrNotFound
	}
	return nil
}

func (r *ImportantDateRepository) ListAll(ctx context.Context) ([]*importantdate.ImportantDate, error) {
	const q = `SELECT ` + importantDateColumns + ` FROM important_dates ORDER BY event_date ASC, display_order ASC`
	return r.list(ctx, q)
}

func (r *ImportantDateRepository) ListActive(ctx context.Context) ([]*importantdate.ImportantDate, error) {
	const q = `SELECT ` + importantDateColumns + ` FROM important_dates WHERE is_active ORDER BY event_date ASC, display_order ASC`
	return r.list(ctx, q)
}

func (r *ImportantDateRepository) list(ctx context.Context, q string) ([]*importantdate.ImportantDate, error) {
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("importantdate_repository: list failed: %w", err)
	}
	defer rows.Close()

	var out []*importantdate.ImportantDate
	for rows.Next() {
		var d importantdate.ImportantDate
		if err := rows.Scan(&d.ID, &d.EventDate, &d.Title, &d.DetailsEN, &d.DetailsBN, &d.DisplayOrder, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("importantdate_repository: scan failed: %w", err)
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("importantdate_repository: list iteration failed: %w", err)
	}
	return out, nil
}
