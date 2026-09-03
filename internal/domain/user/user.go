package user

import "time"

type AuthProvider string

const (
	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

// Role controls access to the admin surface. New accounts are always
// students; an admin is promoted out of band (directly in the database).
type Role string

const (
	RoleStudent Role = "student"
	RoleAdmin   Role = "admin"
)

// User is the core domain entity. PasswordHash and GoogleID are pointers
// because exactly one may be unset depending on how the account was created.
type User struct {
	ID                      string
	Email                   string
	FullName                *string
	PasswordHash            *string
	AuthProvider            AuthProvider
	GoogleID                *string
	ActiveDeviceFingerprint *string
	Role                    Role
	EmailVerified           bool
	// EmailOTP is the plaintext 6-digit code last issued to verify this
	// user's email, or nil once it has been consumed or never issued.
	// EmailOTPExpiry bounds it to a short window (see auth.otpTTL).
	EmailOTP        *string
	EmailOTPExpiry  *time.Time
	InstitutionName *string
	Level           *string
	Medium          *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

func (u *User) HasGoogleLinked() bool {
	return u.GoogleID != nil && *u.GoogleID != ""
}
