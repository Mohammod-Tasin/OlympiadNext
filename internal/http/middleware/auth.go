package middleware

import (
	"context"
	"net/http"
	"strings"

	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/response"
)

type contextKey string

const accessClaimsKey contextKey = "access_claims"

// RequireAccessToken enforces a valid Bearer access token and makes its
// claims available to downstream handlers via context. It also enforces
// single-device sessions: the caller's X-Device-Fingerprint header must
// match the fingerprint stored as the user's active device at last
// login/register, otherwise the token is treated as superseded by a login
// from another device.
func RequireAccessToken(manager *jwt.Manager, users user.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}

			claims, err := manager.ParseAccessToken(raw)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid or expired access token")
				return
			}

			activeFingerprint, err := users.GetActiveDeviceFingerprint(r.Context(), claims.UserID)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid or expired access token")
				return
			}

			requestFingerprint := r.Header.Get("X-Device-Fingerprint")
			if activeFingerprint != "" && requestFingerprint != activeFingerprint {
				response.Error(w, http.StatusUnauthorized, "Session expired or logged in from another device.")
				return
			}

			ctx := context.WithValue(r.Context(), accessClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AccessClaimsFromContext(ctx context.Context) (*jwt.AccessClaims, bool) {
	claims, ok := ctx.Value(accessClaimsKey).(*jwt.AccessClaims)
	return claims, ok
}

func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
