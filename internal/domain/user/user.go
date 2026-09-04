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

// VerificationStatus tracks where an account sits in the manual student
// verification (KYC) review: 'unverified' until a proof document is
// submitted, 'pending' while it waits for an admin, then 'verified' or
// 'rejected' once reviewed. A rejected user may resubmit, returning to
// 'pending'.
type VerificationStatus string

const (
	VerificationUnverified VerificationStatus = "unverified"
	VerificationPending    VerificationStatus = "pending"
	VerificationVerified   VerificationStatus = "verified"
	VerificationRejected   VerificationStatus = "rejected"
)

// Valid reports whether s is one of the four defined states.
func (s VerificationStatus) Valid() bool {
	switch s {
	case VerificationUnverified, VerificationPending, VerificationVerified, VerificationRejected:
		return true
	default:
		return false
	}
}

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
	// ProfilePicture and VerificationDoc hold public URL paths returned by
	// the file-upload endpoint (e.g. "/uploads/users/<id>/<uuid>.pdf").
	// ProfilePicture is optional; VerificationDoc is required to leave the
	// 'unverified' state.
	ProfilePicture     *string
	VerificationDoc    *string
	VerificationStatus VerificationStatus
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// OnboardingProfile is the payload behind PUT /api/user/profile, used for
// both first-time onboarding and later profile edits.
type OnboardingProfile struct {
	FullName        string
	InstitutionName string
	Level           string
	Medium          string
	// VerificationDoc is optional: an empty string keeps the document and
	// verification status already on file; a new value replaces the
	// document and re-opens 'pending' review.
	VerificationDoc string
	ProfilePicture  *string // optional; nil keeps the stored picture
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
