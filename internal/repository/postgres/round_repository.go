package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"olympiadnext/internal/domain/round"
)

const (
	roundColumns       = `id, event_id, round_order, round_name, start_at, duration_minutes, status, created_at, updated_at`
	participantColumns = `id, round_id, user_id, status, decided_at`
)

type RoundRepository struct {
	db *sql.DB
}

func NewRoundRepository(db *sql.DB) *RoundRepository {
	return &RoundRepository{db: db}
}

func (r *RoundRepository) Create(ctx context.Context, rnd *round.Round) error {
	const q = `
		INSERT INTO rounds (id, event_id, round_order, round_name, start_at, duration_minutes, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'upcoming', now(), now())
		RETURNING id, status, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, rnd.EventID, rnd.RoundOrder, rnd.RoundName, rnd.StartAt, rnd.DurationMinutes).
		Scan(&rnd.ID, &rnd.Status, &rnd.CreatedAt, &rnd.UpdatedAt)
	if err != nil {
		if isDuplicateRoundOrder(err) {
			return round.ErrDuplicateOrder
		}
		return fmt.Errorf("round_repository: create failed: %w", err)
	}
	return nil
}

func (r *RoundRepository) Update(ctx context.Context, rnd *round.Round) error {
	const q = `
		UPDATE rounds
		SET round_order = $1, round_name = $2, start_at = $3, duration_minutes = $4, updated_at = now()
		WHERE id = $5
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, q, rnd.RoundOrder, rnd.RoundName, rnd.StartAt, rnd.DurationMinutes, rnd.ID).
		Scan(&rnd.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return round.ErrNotFound
	}
	if err != nil {
		if isDuplicateRoundOrder(err) {
			return round.ErrDuplicateOrder
		}
		return fmt.Errorf("round_repository: update failed: %w", err)
	}
	return nil
}

func (r *RoundRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM rounds WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("round_repository: delete failed: %w", err)
	}
	return checkRoundRowsAffected(res)
}

func (r *RoundRepository) FindByID(ctx context.Context, id string) (*round.Round, error) {
	const q = `SELECT ` + roundColumns + ` FROM rounds WHERE id = $1`
	return scanRound(r.db.QueryRowContext(ctx, q, id))
}

func (r *RoundRepository) FindByEventAndOrder(ctx context.Context, eventID string, order int) (*round.Round, error) {
	const q = `SELECT ` + roundColumns + ` FROM rounds WHERE event_id = $1 AND round_order = $2`
	return scanRound(r.db.QueryRowContext(ctx, q, eventID, order))
}

func (r *RoundRepository) ListByEvent(ctx context.Context, eventID string) ([]*round.Round, error) {
	const q = `SELECT ` + roundColumns + ` FROM rounds WHERE event_id = $1 ORDER BY round_order ASC`

	rows, err := r.db.QueryContext(ctx, q, eventID)
	if err != nil {
		return nil, fmt.Errorf("round_repository: list by event failed: %w", err)
	}
	defer rows.Close()

	var out []*round.Round
	for rows.Next() {
		rnd, err := scanRoundRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rnd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("round_repository: list by event iteration failed: %w", err)
	}
	return out, nil
}

func (r *RoundRepository) MaxRoundOrder(ctx context.Context, eventID string) (int, error) {
	const q = `SELECT COALESCE(MAX(round_order), 0) FROM rounds WHERE event_id = $1`
	var max int
	if err := r.db.QueryRowContext(ctx, q, eventID).Scan(&max); err != nil {
		return 0, fmt.Errorf("round_repository: max round order failed: %w", err)
	}
	return max, nil
}

// SetStatus is a guarded compare-and-swap: it only applies when the round
// is currently in the from state, making forward-only transitions
// concurrency-safe without any app-side branching.
func (r *RoundRepository) SetStatus(ctx context.Context, id string, from, to round.Status) error {
	const q = `UPDATE rounds SET status = $1, updated_at = now() WHERE id = $2 AND status = $3`

	res, err := r.db.ExecContext(ctx, q, to, id, from)
	if err != nil {
		return fmt.Errorf("round_repository: set status failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("round_repository: set status rows affected failed: %w", err)
	}
	if n == 0 {
		if _, findErr := r.FindByID(ctx, id); errors.Is(findErr, round.ErrNotFound) {
			return round.ErrNotFound
		}
		return round.ErrInvalidTransition
	}
	return nil
}

func (r *RoundRepository) GetParticipant(ctx context.Context, roundID, userID string) (*round.Participant, error) {
	const q = `SELECT ` + participantColumns + ` FROM round_participants WHERE round_id = $1 AND user_id = $2`

	var p round.Participant
	err := r.db.QueryRowContext(ctx, q, roundID, userID).Scan(&p.ID, &p.RoundID, &p.UserID, &p.Status, &p.DecidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, round.ErrParticipantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("round_repository: get participant failed: %w", err)
	}
	return &p, nil
}

func (r *RoundRepository) ListParticipantsByRound(ctx context.Context, roundID string) ([]*round.Participant, error) {
	const q = `SELECT ` + participantColumns + ` FROM round_participants WHERE round_id = $1`

	rows, err := r.db.QueryContext(ctx, q, roundID)
	if err != nil {
		return nil, fmt.Errorf("round_repository: list participants by round failed: %w", err)
	}
	defer rows.Close()

	var out []*round.Participant
	for rows.Next() {
		var p round.Participant
		if err := rows.Scan(&p.ID, &p.RoundID, &p.UserID, &p.Status, &p.DecidedAt); err != nil {
			return nil, fmt.Errorf("round_repository: scan participant failed: %w", err)
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("round_repository: list participants by round iteration failed: %w", err)
	}
	return out, nil
}

func (r *RoundRepository) ListParticipantsWithUser(ctx context.Context, roundID string, status round.ParticipantStatus) ([]*round.ParticipantDetail, error) {
	const q = `
		SELECT rp.id, rp.round_id, rp.user_id, rp.status, rp.decided_at, u.full_name, u.email
		FROM round_participants rp
		JOIN users u ON u.id = rp.user_id
		WHERE rp.round_id = $1 AND rp.status = $2`

	rows, err := r.db.QueryContext(ctx, q, roundID, status)
	if err != nil {
		return nil, fmt.Errorf("round_repository: list participants with user failed: %w", err)
	}
	defer rows.Close()

	var out []*round.ParticipantDetail
	for rows.Next() {
		var d round.ParticipantDetail
		if err := rows.Scan(&d.ID, &d.RoundID, &d.UserID, &d.Status, &d.DecidedAt, &d.StudentName, &d.StudentEmail); err != nil {
			return nil, fmt.Errorf("round_repository: scan participant detail failed: %w", err)
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("round_repository: list participants with user iteration failed: %w", err)
	}
	return out, nil
}

// UpsertParticipants records every decision in one transaction so a
// partial failure never leaves the round half-decided.
func (r *RoundRepository) UpsertParticipants(ctx context.Context, roundID string, participants []round.Participant) error {
	if len(participants) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("round_repository: upsert participants begin tx failed: %w", err)
	}
	defer tx.Rollback()

	const q = `
		INSERT INTO round_participants (id, round_id, user_id, status, decided_at)
		VALUES (gen_random_uuid(), $1, $2, $3, now())
		ON CONFLICT (round_id, user_id) DO UPDATE SET status = EXCLUDED.status, decided_at = now()`

	for _, p := range participants {
		if _, err := tx.ExecContext(ctx, q, roundID, p.UserID, p.Status); err != nil {
			return fmt.Errorf("round_repository: upsert participant failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("round_repository: upsert participants commit failed: %w", err)
	}
	return nil
}

func isDuplicateRoundOrder(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolation && pqErr.Constraint == "uq_rounds_event_order"
}

func scanRound(s rowScanner) (*round.Round, error) {
	var rnd round.Round
	err := s.Scan(&rnd.ID, &rnd.EventID, &rnd.RoundOrder, &rnd.RoundName, &rnd.StartAt, &rnd.DurationMinutes, &rnd.Status, &rnd.CreatedAt, &rnd.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, round.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("round_repository: scan failed: %w", err)
	}
	return &rnd, nil
}

func scanRoundRow(rows *sql.Rows) (*round.Round, error) {
	var rnd round.Round
	if err := rows.Scan(&rnd.ID, &rnd.EventID, &rnd.RoundOrder, &rnd.RoundName, &rnd.StartAt, &rnd.DurationMinutes, &rnd.Status, &rnd.CreatedAt, &rnd.UpdatedAt); err != nil {
		return nil, fmt.Errorf("round_repository: scan row failed: %w", err)
	}
	return &rnd, nil
}

func checkRoundRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("round_repository: rows affected failed: %w", err)
	}
	if n == 0 {
		return round.ErrNotFound
	}
	return nil
}
