package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"olympiadnext/internal/domain/registration"
)

// The two column lists below must stay in the same order as
// scanRegistrationInto / scanDetailRow. registrationColumns is the bare
// projection for single-table reads; registrationColumnsER is the same
// list aliased for the JOIN queries that also pull the event title and the
// student's name/email.
const (
	registrationColumns   = `id, user_id, event_id, payment_method, sender_number, transaction_id, status, reviewed_by, reviewed_at, created_at, updated_at`
	registrationColumnsER = `er.id, er.user_id, er.event_id, er.payment_method, er.sender_number, er.transaction_id, er.status, er.reviewed_by, er.reviewed_at, er.created_at, er.updated_at`
)

type RegistrationRepository struct {
	db *sql.DB
}

func NewRegistrationRepository(db *sql.DB) *RegistrationRepository {
	return &RegistrationRepository{db: db}
}

func (r *RegistrationRepository) Create(ctx context.Context, reg *registration.Registration) error {
	const q = `
		INSERT INTO exam_registrations (id, user_id, event_id, payment_method, sender_number, transaction_id, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'pending', now(), now())
		RETURNING id, status, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, reg.UserID, reg.EventID, reg.PaymentMethod, reg.SenderNumber, reg.TransactionID).
		Scan(&reg.ID, &reg.Status, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolation {
			switch pqErr.Constraint {
			case "uq_exam_reg_transaction_id":
				return registration.ErrDuplicateTransactionID
			case "uq_exam_reg_user_event":
				return registration.ErrAlreadyRegistered
			}
		}
		return fmt.Errorf("registration_repository: create failed: %w", err)
	}
	return nil
}

func (r *RegistrationRepository) FindByID(ctx context.Context, id string) (*registration.Registration, error) {
	const q = `SELECT ` + registrationColumns + ` FROM exam_registrations WHERE id = $1`

	var reg registration.Registration
	if err := scanRegistrationInto(r.db.QueryRowContext(ctx, q, id), &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

func (r *RegistrationRepository) ListByUser(ctx context.Context, userID string) ([]*registration.Detail, error) {
	const q = `
		SELECT ` + registrationColumnsER + `, e.title
		FROM exam_registrations er
		JOIN events e ON e.id = er.event_id
		WHERE er.user_id = $1
		ORDER BY er.created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("registration_repository: list by user failed: %w", err)
	}
	defer rows.Close()

	var out []*registration.Detail
	for rows.Next() {
		var d registration.Detail
		if err := scanDetailRow(rows, &d, false); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("registration_repository: list by user iteration failed: %w", err)
	}
	return out, nil
}

func (r *RegistrationRepository) ListByStatus(ctx context.Context, status registration.Status, limit int) ([]*registration.Detail, error) {
	// The JOIN and projection are identical; only the status filter differs.
	base := `
		SELECT ` + registrationColumnsER + `, e.title, u.full_name, u.email
		FROM exam_registrations er
		JOIN events e ON e.id = er.event_id
		JOIN users u ON u.id = er.user_id`

	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = r.db.QueryContext(ctx, base+` ORDER BY er.created_at DESC LIMIT $1`, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, base+` WHERE er.status = $1 ORDER BY er.created_at DESC LIMIT $2`, status, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("registration_repository: list by status failed: %w", err)
	}
	defer rows.Close()

	var out []*registration.Detail
	for rows.Next() {
		var d registration.Detail
		if err := scanDetailRow(rows, &d, true); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("registration_repository: list by status iteration failed: %w", err)
	}
	return out, nil
}

// Review flips a still-pending row to the admin's decision. The
// `status = 'pending'` guard makes a double review a no-op that surfaces
// as ErrAlreadyReviewed rather than silently overwriting an earlier
// decision or its reviewer.
func (r *RegistrationRepository) Review(ctx context.Context, id, reviewedBy string, status registration.Status) error {
	const q = `
		UPDATE exam_registrations
		SET status = $1, reviewed_by = $2, reviewed_at = now(), updated_at = now()
		WHERE id = $3 AND status = 'pending'`

	res, err := r.db.ExecContext(ctx, q, status, reviewedBy, id)
	if err != nil {
		return fmt.Errorf("registration_repository: review failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration_repository: review rows affected failed: %w", err)
	}
	if n == 0 {
		// Either the id is unknown or the row is no longer pending; one
		// lookup tells the two apart so the handler can 404 vs 409.
		if _, findErr := r.FindByID(ctx, id); errors.Is(findErr, registration.ErrNotFound) {
			return registration.ErrNotFound
		}
		return registration.ErrAlreadyReviewed
	}
	return nil
}

// Unreject is deliberately a separate, tightly scoped correction from
// Review. Its WHERE clause is the concurrency-safe transition guard, and it
// clears both review fields because the row is once again awaiting review.
func (r *RegistrationRepository) Unreject(ctx context.Context, id string) error {
	const q = `
		UPDATE exam_registrations
		SET status = 'pending', reviewed_by = NULL, reviewed_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'rejected'`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("registration_repository: unreject failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration_repository: unreject rows affected failed: %w", err)
	}
	if n == 0 {
		if _, findErr := r.FindByID(ctx, id); errors.Is(findErr, registration.ErrNotFound) {
			return registration.ErrNotFound
		}
		return registration.ErrInvalidUnrejectTransition
	}
	return nil
}

func scanRegistrationInto(s rowScanner, reg *registration.Registration) error {
	err := s.Scan(
		&reg.ID, &reg.UserID, &reg.EventID, &reg.PaymentMethod, &reg.SenderNumber,
		&reg.TransactionID, &reg.Status, &reg.ReviewedBy, &reg.ReviewedAt,
		&reg.CreatedAt, &reg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return registration.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("registration_repository: scan failed: %w", err)
	}
	return nil
}

// scanDetailRow scans the aliased registration columns plus the joined
// event title and, when withStudent is true, the student's name and email.
func scanDetailRow(rows *sql.Rows, d *registration.Detail, withStudent bool) error {
	dest := []any{
		&d.ID, &d.UserID, &d.EventID, &d.PaymentMethod, &d.SenderNumber,
		&d.TransactionID, &d.Status, &d.ReviewedBy, &d.ReviewedAt,
		&d.CreatedAt, &d.UpdatedAt, &d.EventTitle,
	}
	if withStudent {
		dest = append(dest, &d.StudentName, &d.StudentEmail)
	}
	if err := rows.Scan(dest...); err != nil {
		return fmt.Errorf("registration_repository: scan detail failed: %w", err)
	}
	return nil
}
