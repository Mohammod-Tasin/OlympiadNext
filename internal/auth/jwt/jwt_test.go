package jwt

import (
	"testing"
	"time"
)

// TestTokens_HaveUniqueJTI is the root-cause guard for the token_hash
// collision: two tokens minted for the same user in the same instant must
// differ, and the random `jti` claim is what guarantees it.
func TestTokens_HaveUniqueJTI(t *testing.T) {
	m := NewManager("access-secret", "refresh-secret", 15*time.Minute, 720*time.Hour)

	a1, err := m.GenerateAccessToken("user-1", "user@example.com")
	if err != nil {
		t.Fatalf("access token 1: %v", err)
	}
	a2, err := m.GenerateAccessToken("user-1", "user@example.com")
	if err != nil {
		t.Fatalf("access token 2: %v", err)
	}
	if a1 == a2 {
		t.Fatal("two access tokens for the same user in the same second are byte-identical")
	}

	r1, _, err := m.GenerateRefreshToken("user-1")
	if err != nil {
		t.Fatalf("refresh token 1: %v", err)
	}
	r2, _, err := m.GenerateRefreshToken("user-1")
	if err != nil {
		t.Fatalf("refresh token 2: %v", err)
	}
	if r1 == r2 {
		t.Fatal("two refresh tokens for the same user in the same second are byte-identical")
	}
}

// TestParse_AcceptsJTIClaim confirms the parsers still validate a token
// that now carries a `jti`, and that the claim round-trips.
func TestParse_AcceptsJTIClaim(t *testing.T) {
	m := NewManager("access-secret", "refresh-secret", 15*time.Minute, 720*time.Hour)

	access, err := m.GenerateAccessToken("user-1", "user@example.com")
	if err != nil {
		t.Fatalf("generate access: %v", err)
	}
	ac, err := m.ParseAccessToken(access)
	if err != nil {
		t.Fatalf("parse access token with jti: %v", err)
	}
	if ac.ID == "" {
		t.Fatal("access claims carry no jti")
	}
	if ac.UserID != "user-1" || ac.Email != "user@example.com" {
		t.Fatalf("access claims did not round-trip: %+v", ac)
	}

	refresh, _, err := m.GenerateRefreshToken("user-1")
	if err != nil {
		t.Fatalf("generate refresh: %v", err)
	}
	rc, err := m.ParseRefreshToken(refresh)
	if err != nil {
		t.Fatalf("parse refresh token with jti: %v", err)
	}
	if rc.ID == "" {
		t.Fatal("refresh claims carry no jti")
	}
	if rc.UserID != "user-1" {
		t.Fatalf("refresh claims did not round-trip: %+v", rc)
	}
}
