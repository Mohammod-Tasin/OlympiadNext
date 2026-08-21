package token

import (
	"context"
	"time"
)

// RefreshToken is stored server-side (hashed, never the raw token) so a
// session can be revoked or rotated without waiting for JWT expiry.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
	Revoked   bool
}

func (r *RefreshToken) IsActive(now time.Time) bool {
	return r.RevokedAt == nil && now.Before(r.ExpiresAt)
}

type Repository interface {
	Create(ctx context.Context, t *RefreshToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}
