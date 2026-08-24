package user

import "context"

// Repository abstracts persistence for User so the application layer
// never depends on a concrete database driver.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	LinkGoogleID(ctx context.Context, userID, googleID string) error
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	UpdateActiveDeviceFingerprint(ctx context.Context, userID, deviceFingerprint string) error
	GetActiveDeviceFingerprint(ctx context.Context, userID string) (string, error)
	MarkEmailVerified(ctx context.Context, userID string) error
	MarkPhoneVerified(ctx context.Context, userID string) error
	UpdatePhoneNumber(ctx context.Context, userID, phone string) error
}
