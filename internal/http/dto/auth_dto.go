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

type AuthResponse struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}
