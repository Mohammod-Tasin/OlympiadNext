package user

import (
	"context"
	"time"
)

// Repository abstracts persistence for User so the application layer
// never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	LinkGoogleID(ctx context.Context, userID, googleID string, isEmailVerified bool) error
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	UpdateActiveDeviceFingerprint(ctx context.Context, userID, deviceFingerprint string) error
	GetActiveDeviceFingerprint(ctx context.Context, userID string) (string, error)
	GetRole(ctx context.Context, userID string) (Role, error)
	// SetEmailOTP stores a freshly generated verification code and its
	// expiry, replacing any code previously issued to the user.
	SetEmailOTP(ctx context.Context, userID, code string, expiresAt time.Time) error
	// MarkEmailVerified flips email_verified to true and clears any
	// outstanding OTP so a spent code can never be replayed.
	MarkEmailVerified(ctx context.Context, userID string) error
	UpdateAcademicProfile(ctx context.Context, userID string, fullName, institution, level, medium string) error
}
