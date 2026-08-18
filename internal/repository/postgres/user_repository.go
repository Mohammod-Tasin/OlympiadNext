package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"olympiadnext/internal/domain/user"
)

const pgUniqueViolation = "23505"

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	const q = `
		INSERT INTO users (id, email, password_hash, auth_provider, google_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, now(), now())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, u.Email, u.PasswordHash, u.AuthProvider, u.GoogleID).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolation {
			return user.ErrEmailTaken
		}
		return fmt.Errorf("user_repository: create failed: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	const q = `
		SELECT id, email, password_hash, auth_provider, google_id, created_at, updated_at
		FROM users WHERE id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	const q = `
		SELECT id, email, password_hash, auth_provider, google_id, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)`
	return r.scanOne(r.db.QueryRowContext(ctx, q, email))
}

func (r *UserRepository) FindByGoogleID(ctx context.Context, googleID string) (*user.User, error) {
	const q = `
		SELECT id, email, password_hash, auth_provider, google_id, created_at, updated_at
		FROM users WHERE google_id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, googleID))
}

func (r *UserRepository) LinkGoogleID(ctx context.Context, userID, googleID string) error {
	const q = `UPDATE users SET google_id = $1, updated_at = now() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, q, googleID, userID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolation {
			return user.ErrGoogleIDTaken
		}
		return fmt.Errorf("user_repository: link google id failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	const q = `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, q, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("user_repository: update password failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) scanOne(row *sql.Row) (*user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.AuthProvider, &u.GoogleID, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repository: scan failed: %w", err)
	}
	return &u, nil
}

func checkRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("user_repository: rows affected failed: %w", err)
	}
	if n == 0 {
		return user.ErrNotFound
	}
	return nil
}
