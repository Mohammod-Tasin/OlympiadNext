package user

import "time"

type AuthProvider string

const (
	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

// User is the core domain entity. PasswordHash and GoogleID are pointers
// because exactly one may be unset depending on how the account was created.
//
// EmailOTP/EmailOTPExpiry hold the single outstanding email verification
// code: issuing a new code overwrites the old one, and verifying clears
// both, so a user never has more than one live code.
type User struct {
	ID                      string
	Email                   string
	FullName                *string
	PasswordHash            *string
	AuthProvider            AuthProvider
	GoogleID                *string
	ActiveDeviceFingerprint *string
	EmailVerified           bool
	EmailOTP                *string
	EmailOTPExpiry          *time.Time
	InstitutionName         *string
	Level                   *string
	Medium                  *string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

func (u *User) HasGoogleLinked() bool {
	return u.GoogleID != nil && *u.GoogleID != ""
}
