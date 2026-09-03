package dto

import "time"

type RegisterRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	FullName        string `json:"full_name"`
	InstitutionName string `json:"institution_name"`
	Level           string `json:"level"`
	Medium          string `json:"medium"`
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

type UpdateAcademicProfileRequest struct {
	FullName        string `json:"full_name"`
	InstitutionName string `json:"institution_name"`
	Level           string `json:"level"`
	Medium          string `json:"medium"`
}

type AuthResponse struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}

type UserResponse struct {
	UserID          string  `json:"user_id"`
	Email           string  `json:"email"`
	FullName        *string `json:"full_name,omitempty"`
	EmailVerified   bool    `json:"email_verified"`
	InstitutionName *string `json:"institution_name,omitempty"`
	Level           *string `json:"level,omitempty"`
	Medium          *string `json:"medium,omitempty"`
}
