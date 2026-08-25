package otp

import (
	"context"
	"time"
)

type TargetType string

const (
	TargetEmail TargetType = "email"
	TargetPhone TargetType = "phone"
)

// OTP is a one-time code issued to verify a user's email or phone. Codes
// are single-use: a successful verification deletes the row rather than
// flagging it, so "latest valid OTP" never has to filter out spent ones.
type OTP struct {
	ID         string
	UserID     string
	TargetType TargetType
	Code       string // SHA-256 hex digest of the delivered code, never the plaintext
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

type Repository interface {
	Create(ctx context.Context, o *OTP) error
	FindLatestValid(ctx context.Context, userID string, targetType TargetType) (*OTP, error)
	Delete(ctx context.Context, id string) error
}
