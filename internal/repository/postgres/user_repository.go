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

// userColumns is the full projection scanned by scanUser, in struct-field
// order. Every SELECT that feeds scanUser must use exactly this list.
const userColumns = `id, email, full_name, password_hash, auth_provider, google_id, active_device_fingerprint, role, email_verified, email_otp, email_otp_expiry, institution_name, level, medium, profile_picture, verification_doc, verification_status, created_at, updated_at`

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
		RETURNING id, role, email_verified, verification_status, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, u.Email, u.FullName, u.PasswordHash, u.AuthProvider, u.GoogleID, u.EmailVerified, u.InstitutionName, u.Level, u.Medium).
		Scan(&u.ID, &u.Role, &u.EmailVerified, &u.VerificationStatus, &u.CreatedAt, &u.UpdatedAt)
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
	const q = `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	return scanUser(r.db.QueryRowContext(ctx, q, id))
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE LOWER(email) = LOWER($1)`
	return scanUser(r.db.QueryRowContext(ctx, q, email))
}

func (r *UserRepository) FindByGoogleID(ctx context.Context, googleID string) (*user.User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE google_id = $1`
	return scanUser(r.db.QueryRowContext(ctx, q, googleID))
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

// SubmitOnboardingProfile writes the profile fields and returns the
// account's resulting verification status. It backs both first-time
// onboarding and later profile edits: a non-empty p.VerificationDoc
// replaces the stored document and moves the account to 'pending', while
// an empty one retains the existing document and status so an already
// verified user can edit their name without re-entering review. A nil
// ProfilePicture leaves the existing one untouched.
func (r *UserRepository) SubmitOnboardingProfile(ctx context.Context, userID string, p user.OnboardingProfile) (user.VerificationStatus, error) {
	const q = `
		UPDATE users
		SET full_name = $1,
		    institution_name = $2,
		    level = $3,
		    medium = $4,
		    profile_picture = COALESCE($5, profile_picture),
		    verification_doc = CASE WHEN $6 <> '' THEN $6 ELSE verification_doc END,
		    verification_status = CASE WHEN $6 <> '' THEN 'pending' ELSE verification_status END,
		    updated_at = now()
		WHERE id = $7
		RETURNING verification_status`
	var status user.VerificationStatus
	err := r.db.QueryRowContext(ctx, q, p.FullName, p.InstitutionName, p.Level, p.Medium, p.ProfilePicture, p.VerificationDoc, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", user.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("user_repository: submit onboarding profile failed: %w", err)
	}
	return status, nil
}

// ListUsers returns users newest-first. status "" returns every user;
// otherwise only rows in that verification state. limit bounds the result.
func (r *UserRepository) ListUsers(ctx context.Context, status user.VerificationStatus, limit int) ([]*user.User, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		const q = `SELECT ` + userColumns + ` FROM users ORDER BY created_at DESC LIMIT $1`
		rows, err = r.db.QueryContext(ctx, q, limit)
	} else {
		const q = `SELECT ` + userColumns + ` FROM users WHERE verification_status = $1 ORDER BY created_at DESC LIMIT $2`
		rows, err = r.db.QueryContext(ctx, q, status, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("user_repository: list users failed: %w", err)
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		u, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user_repository: list users iteration failed: %w", err)
	}
	return users, nil
}

// SetVerificationStatus records an admin's approve/reject decision.
func (r *UserRepository) SetVerificationStatus(ctx context.Context, userID string, status user.VerificationStatus) error {
	const q = `UPDATE users SET verification_status = $1, updated_at = now() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, q, status, userID)
	if err != nil {
		return fmt.Errorf("user_repository: set verification status failed: %w", err)
	}
	return checkRowsAffected(res)
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, letting scanUser
// serve single-row lookups and list iteration alike.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(s rowScanner) (*user.User, error) {
	var u user.User
	err := s.Scan(
		&u.ID, &u.Email, &u.FullName, &u.PasswordHash, &u.AuthProvider, &u.GoogleID,
		&u.ActiveDeviceFingerprint, &u.Role, &u.EmailVerified, &u.EmailOTP, &u.EmailOTPExpiry,
		&u.InstitutionName, &u.Level, &u.Medium, &u.ProfilePicture, &u.VerificationDoc,
		&u.VerificationStatus, &u.CreatedAt, &u.UpdatedAt,
	)
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
