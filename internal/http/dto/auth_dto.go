package dto

import "time"

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email             string `json:"email"`
	Password          string `json:"password"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
}

type GoogleLoginRequest struct {
	IDToken           string `json:"id_token"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
}

type VerifyEmailOTPRequest struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

type ResendEmailOTPRequest struct {
	Email string `json:"email"`
}

type AuthResponse struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}

type UserResponse struct {
	UserID             string  `json:"user_id"`
	Email              string  `json:"email"`
	FullName           *string `json:"full_name,omitempty"`
	EmailVerified      bool    `json:"email_verified"`
	InstitutionName    *string `json:"institution_name,omitempty"`
	Level              *string `json:"level,omitempty"`
	Medium             *string `json:"medium,omitempty"`
	ProfilePicture     *string `json:"profile_picture,omitempty"`
	VerificationDoc    *string `json:"verification_doc,omitempty"`
	VerificationStatus string  `json:"verification_status"`
}
