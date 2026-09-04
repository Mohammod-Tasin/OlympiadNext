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
	// SubmitOnboardingProfile writes the academic fields and KYC file
	// references and moves the account to 'pending' review in one UPDATE.
	SubmitOnboardingProfile(ctx context.Context, userID string, p OnboardingProfile) error
	// ListUsers returns users newest-first. A non-empty status filters to
	// that verification state; limit caps the row count.
	ListUsers(ctx context.Context, status VerificationStatus, limit int) ([]*User, error)
	// SetVerificationStatus records an admin's review decision.
	SetVerificationStatus(ctx context.Context, userID string, status VerificationStatus) error
}
