// Package google verifies Google-issued ID tokens on the backend so the
// server never trusts client-asserted identity directly.
package google

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/idtoken"
)

var ErrInvalidToken = errors.New("google: invalid id token")

// Claims is the subset of the verified Google ID token payload the
// application cares about.
type Claims struct {
	Subject       string // Google's stable per-user "sub" claim
	Email         string
	EmailVerified bool
}

type Verifier struct {
	clientID string
}

func NewVerifier(clientID string) *Verifier {
	return &Verifier{clientID: clientID}
}

// Verify validates signature, issuer, expiry and audience (our OAuth
// client ID) against Google's published certificates.
func (v *Verifier) Verify(ctx context.Context, rawIDToken string) (*Claims, error) {
	payload, err := idtoken.Validate(ctx, rawIDToken, v.clientID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	sub, _ := payload.Claims["sub"].(string)
	emailClaim, _ := payload.Claims["email"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)

	if sub == "" || emailClaim == "" {
		return nil, ErrInvalidToken
	}

	return &Claims{
		Subject:       sub,
		Email:         emailClaim,
		EmailVerified: emailVerified,
	}, nil
}
