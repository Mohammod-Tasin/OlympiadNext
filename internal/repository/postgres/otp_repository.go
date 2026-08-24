package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/otp"
)

type OTPRepository struct {
	db *sql.DB
}

func NewOTPRepository(db *sql.DB) *OTPRepository {
	return &OTPRepository{db: db}
}

func (r *OTPRepository) Create(ctx context.Context, o *otp.OTP) error {
	const q = `
		INSERT INTO otps (id, user_id, target_type, code, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, now())
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, q, o.UserID, o.TargetType, o.Code, o.ExpiresAt).
		Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return fmt.Errorf("otp_repository: create failed: %w", err)
	}
	return nil
}

func (r *OTPRepository) FindLatestValid(ctx context.Context, userID string, targetType otp.TargetType) (*otp.OTP, error) {
	const q = `
		SELECT id, user_id, target_type, code, expires_at, created_at
		FROM otps
		WHERE user_id = $1 AND target_type = $2 AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1`

	var o otp.OTP
	err := r.db.QueryRowContext(ctx, q, userID, targetType).
		Scan(&o.ID, &o.UserID, &o.TargetType, &o.Code, &o.ExpiresAt, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, otp.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("otp_repository: find latest valid failed: %w", err)
	}
	return &o, nil
}

func (r *OTPRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM otps WHERE id = $1`
	if _, err := r.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("otp_repository: delete failed: %w", err)
	}
	return nil
}
