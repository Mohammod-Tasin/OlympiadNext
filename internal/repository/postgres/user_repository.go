package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		INSERT INTO users (id, email, full_name, password_hash, auth_provider, google_id, email_verified, institution_name, level, medium, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
		RETURNING id, role, email_verified, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, u.Email, u.FullName, u.PasswordHash, u.AuthProvider, u.GoogleID, u.EmailVerified, u.InstitutionName, u.Level, u.Medium).
		Scan(&u.ID, &u.Role, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt)
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
		SELECT id, email, full_name, password_hash, auth_provider, google_id, active_device_fingerprint, role, email_verified, email_otp, email_otp_expiry, institution_name, level, medium, created_at, updated_at
		FROM users WHERE id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	const q = `
		SELECT id, email, full_name, password_hash, auth_provider, google_id, active_device_fingerprint, role, email_verified, email_otp, email_otp_expiry, institution_name, level, medium, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)`
	return r.scanOne(r.db.QueryRowContext(ctx, q, email))
}

func (r *UserRepository) FindByGoogleID(ctx context.Context, googleID string) (*user.User, error) {
	const q = `
		SELECT id, email, full_name, password_hash, auth_provider, google_id, active_device_fingerprint, role, email_verified, email_otp, email_otp_expiry, institution_name, level, medium, created_at, updated_at
		FROM users WHERE google_id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, googleID))
}

func (r *UserRepository) LinkGoogleID(ctx context.Context, userID, googleID string, isEmailVerified bool) error {
	const q = `UPDATE users SET google_id = $1, email_verified = $2, updated_at = now() WHERE id = $3`
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

// GetRole returns just the caller's role, for the admin-gate middleware
// which has no need to load the full user row.
func (r *UserRepository) GetRole(ctx context.Context, userID string) (user.Role, error) {
	const q = `SELECT role FROM users WHERE id = $1`
	var role user.Role
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", user.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("user_repository: get role failed: %w", err)
	}
	return role, nil
}

// SetEmailOTP stores a freshly generated verification code and its expiry
// on the user row, overwriting any code issued earlier.
func (r *UserRepository) SetEmailOTP(ctx context.Context, userID, code string, expiresAt time.Time) error {
	const q = `UPDATE users SET email_otp = $1, email_otp_expiry = $2, updated_at = now() WHERE id = $3`
	res, err := r.db.ExecContext(ctx, q, code, expiresAt, userID)
	if err != nil {
		return fmt.Errorf("user_repository: set email otp failed: %w", err)
	}
	return checkRowsAffected(res)
}

// MarkEmailVerified flips email_verified and clears the OTP columns in one
// statement so a consumed code cannot be replayed.
func (r *UserRepository) MarkEmailVerified(ctx context.Context, userID string) error {
	const q = `UPDATE users SET email_verified = true, email_otp = NULL, email_otp_expiry = NULL, updated_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repository: mark email verified failed: %w", err)
	}
	return checkRowsAffected(res)
}

// UpdateAcademicProfile sets the caller's full name, institution, academic
// level, and medium of instruction.
func (r *UserRepository) UpdateAcademicProfile(ctx context.Context, userID string, fullName, institution, level, medium string) error {
	const q = `UPDATE users SET full_name = $1, institution_name = $2, level = $3, medium = $4, updated_at = now() WHERE id = $5`
	res, err := r.db.ExecContext(ctx, q, fullName, institution, level, medium, userID)
	if err != nil {
		return fmt.Errorf("user_repository: update academic profile failed: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) scanOne(row *sql.Row) (*user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.PasswordHash, &u.AuthProvider, &u.GoogleID, &u.ActiveDeviceFingerprint, &u.Role, &u.EmailVerified, &u.EmailOTP, &u.EmailOTPExpiry, &u.InstitutionName, &u.Level, &u.Medium, &u.CreatedAt, &u.UpdatedAt)
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
