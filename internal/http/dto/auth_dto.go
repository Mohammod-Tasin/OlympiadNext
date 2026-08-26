package dto

import "time"

type RegisterRequest struct {
	Email             string `json:"email"`
	Password          string `json:"password"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
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

type SendOTPRequest struct {
	Type string `json:"type"`
}

type VerifyOTPRequest struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

type UpdatePhoneNumberRequest struct {
	PhoneNumber string `json:"phone_number"`
}

type UpdateAcademicProfileRequest struct {
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
	PhoneNumber     *string `json:"phone_number,omitempty"`
	IsEmailVerified bool    `json:"is_email_verified"`
	IsPhoneVerified bool    `json:"is_phone_verified"`
	InstitutionName *string `json:"institution_name,omitempty"`
	Level           *string `json:"level,omitempty"`
	Medium          *string `json:"medium,omitempty"`
}
