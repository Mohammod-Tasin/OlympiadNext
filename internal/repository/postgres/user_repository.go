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
		INSERT INTO users (id, email, password_hash, auth_provider, google_id, is_email_verified, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
		RETURNING id, is_email_verified, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, u.Email, u.PasswordHash, u.AuthProvider, u.GoogleID, u.IsEmailVerified).
		Scan(&u.ID, &u.IsEmailVerified, &u.CreatedAt, &u.UpdatedAt)
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
		SELECT id, email, password_hash, auth_provider, google_id, phone_number, active_device_fingerprint, is_email_verified, is_phone_verified, institution_name, level, medium, created_at, updated_at
		FROM users WHERE id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	const q = `
		SELECT id, email, password_hash, auth_provider, google_id, phone_number, active_device_fingerprint, is_email_verified, is_phone_verified, institution_name, level, medium, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)`
	return r.scanOne(r.db.QueryRowContext(ctx, q, email))
}

func (r *UserRepository) FindByGoogleID(ctx context.Context, googleID string) (*user.User, error) {
	const q = `
		SELECT id, email, password_hash, auth_provider, google_id, phone_number, active_device_fingerprint, is_email_verified, is_phone_verified, institution_name, level, medium, created_at, updated_at
		FROM users WHERE google_id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, googleID))
}

func (r *UserRepository) LinkGoogleID(ctx context.Context, userID, googleID string, isEmailVerified bool) error {
	const q = `UPDATE users SET google_id = $1, is_email_verified = $2, updated_at = now() WHERE id = $3`
	res, err := r.db.ExecContext(ctx, q, googleID, isEmailVerified, userID)
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

func (r *UserRepository) UpdateActiveDeviceFingerprint(ctx context.Context, userID, deviceFingerprint string) error {
	const q = `UPDATE users SET active_device_fingerprint = $1, updated_at = now() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, q, deviceFingerprint, userID)
	if err != nil {
		return fmt.Errorf("user_repository: update active device fingerprint failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) GetActiveDeviceFingerprint(ctx context.Context, userID string) (string, error) {
	const q = `SELECT active_device_fingerprint FROM users WHERE id = $1`
	var fingerprint sql.NullString
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", user.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("user_repository: get active device fingerprint failed: %w", err)
	}
	return fingerprint.String, nil
}

func (r *UserRepository) MarkEmailVerified(ctx context.Context, userID string) error {
	const q = `UPDATE users SET is_email_verified = true, updated_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repository: mark email verified failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) MarkPhoneVerified(ctx context.Context, userID string) error {
	const q = `UPDATE users SET is_phone_verified = true, updated_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repository: mark phone verified failed: %w", err)
	}
	return checkRowsAffected(res)
}

// UpdateAcademicProfile sets the caller's institution, academic level,
// and medium of instruction.
func (r *UserRepository) UpdateAcademicProfile(ctx context.Context, userID string, institution, level, medium string) error {
	const q = `UPDATE users SET institution_name = $1, level = $2, medium = $3, updated_at = now() WHERE id = $4`
	res, err := r.db.ExecContext(ctx, q, institution, level, medium, userID)
	if err != nil {
		return fmt.Errorf("user_repository: update academic profile failed: %w", err)
	}
	return checkRowsAffected(res)
}

// UpdatePhoneNumber sets the phone number and always resets verification:
// a changed number has never had an OTP delivered to it, so any prior
// verified status no longer applies.
func (r *UserRepository) UpdatePhoneNumber(ctx context.Context, userID, phone string) error {
	const q = `UPDATE users SET phone_number = $1, is_phone_verified = false, updated_at = now() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, q, phone, userID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolation {
			return user.ErrPhoneTaken
		}
		return fmt.Errorf("user_repository: update phone number failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) scanOne(row *sql.Row) (*user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.AuthProvider, &u.GoogleID, &u.PhoneNumber, &u.ActiveDeviceFingerprint, &u.IsEmailVerified, &u.IsPhoneVerified, &u.InstitutionName, &u.Level, &u.Medium, &u.CreatedAt, &u.UpdatedAt)
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
