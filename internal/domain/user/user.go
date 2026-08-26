package user

import "time"

type AuthProvider string

const (
	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

// User is the core domain entity. PasswordHash and GoogleID are pointers
// because exactly one may be unset depending on how the account was created.
type User struct {
	ID                      string
	Email                   string
	PasswordHash            *string
	AuthProvider            AuthProvider
	GoogleID                *string
	PhoneNumber             *string
	ActiveDeviceFingerprint *string
	IsEmailVerified         bool
	IsPhoneVerified         bool
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
