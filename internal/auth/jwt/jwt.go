// Package jwt issues and validates the two token types used for
// sessions: short-lived access tokens and long-lived refresh tokens,
// each signed with its own secret so a leaked access token cannot be
// replayed as a refresh token.
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("jwt: invalid or expired token")
)

type AccessClaims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (m *Manager) AccessTokenTTL() time.Duration  { return m.accessTTL }
func (m *Manager) RefreshTokenTTL() time.Duration { return m.refreshTTL }

func (m *Manager) GenerateAccessToken(userID, email string) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.accessSecret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign access token failed: %w", err)
	}
	return token, nil
}

func (m *Manager) GenerateRefreshToken(userID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.refreshTTL)
	claims := RefreshClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.refreshSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: sign refresh token failed: %w", err)
	}
	return token, expiresAt, nil
}

func (m *Manager) ParseAccessToken(raw string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	if err := m.parse(raw, claims, m.accessSecret); err != nil {
		return nil, err
	}
	return claims, nil
}

func (m *Manager) ParseRefreshToken(raw string) (*RefreshClaims, error) {
	claims := &RefreshClaims{}
	if err := m.parse(raw, claims, m.refreshSecret); err != nil {
		return nil, err
	}
	return claims, nil
}

func (m *Manager) parse(raw string, claims jwt.Claims, secret []byte) error {
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))

	if err != nil || !token.Valid {
		return ErrInvalidToken
	}
	return nil
}
