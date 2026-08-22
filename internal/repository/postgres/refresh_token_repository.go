package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/token"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *token.RefreshToken) error {
	const q = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, now())
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, q, t.UserID, t.TokenHash, t.ExpiresAt).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return fmt.Errorf("refresh_token_repository: create failed: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*token.RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, revoked
		FROM refresh_tokens WHERE token_hash = $1`

	var t token.RefreshToken
	err := r.db.QueryRowContext(ctx, q, tokenHash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt, &t.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("refresh_token_repository: find failed: %w", err)
	}
	return &t, nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, id string) (bool, error) {
	const q = `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return false, fmt.Errorf("refresh_token_repository: revoke failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("refresh_token_repository: revoke rows affected failed: %w", err)
	}
	return n == 1, nil
}

func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	const q = `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := r.db.ExecContext(ctx, q, userID); err != nil {
		return fmt.Errorf("refresh_token_repository: revoke all failed: %w", err)
	}
	return nil
}
