package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/prize"
)

const prizeColumns = `id, event_id, rank_from, rank_to, prize_name, prize_description, level, created_at, updated_at`

type PrizeRepository struct {
	db *sql.DB
}

func NewPrizeRepository(db *sql.DB) *PrizeRepository {
	return &PrizeRepository{db: db}
}

func (r *PrizeRepository) Create(ctx context.Context, p *prize.Prize) error {
	const q = `
		INSERT INTO prizes (id, event_id, rank_from, rank_to, prize_name, prize_description, level, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, now(), now())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, p.EventID, p.RankFrom, p.RankTo, p.PrizeName, p.PrizeDescription, p.Level).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("prize_repository: create failed: %w", err)
	}
	return nil
}

func (r *PrizeRepository) Update(ctx context.Context, p *prize.Prize) error {
	const q = `
		UPDATE prizes
		SET rank_from = $1, rank_to = $2, prize_name = $3, prize_description = $4, level = $5, updated_at = now()
		WHERE id = $6
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, q, p.RankFrom, p.RankTo, p.PrizeName, p.PrizeDescription, p.Level, p.ID).
		Scan(&p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return prize.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("prize_repository: update failed: %w", err)
	}
	return nil
}

func (r *PrizeRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM prizes WHERE id = $1`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("prize_repository: delete failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("prize_repository: delete rows affected failed: %w", err)
	}
	if n == 0 {
		return prize.ErrNotFound
	}
	return nil
}

func (r *PrizeRepository) ListByEvent(ctx context.Context, eventID string) ([]*prize.Prize, error) {
	const q = `SELECT ` + prizeColumns + ` FROM prizes WHERE event_id = $1 ORDER BY rank_from ASC`

	rows, err := r.db.QueryContext(ctx, q, eventID)
	if err != nil {
		return nil, fmt.Errorf("prize_repository: list by event failed: %w", err)
	}
	defer rows.Close()

	var out []*prize.Prize
	for rows.Next() {
		var p prize.Prize
		if err := rows.Scan(&p.ID, &p.EventID, &p.RankFrom, &p.RankTo, &p.PrizeName, &p.PrizeDescription, &p.Level, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("prize_repository: scan failed: %w", err)
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prize_repository: list by event iteration failed: %w", err)
	}
	return out, nil
}
