package middleware

import (
	"net/http"
	"slices"

	"olympiadnext/internal/http/response"
)

// RequireTrustedOrigin rejects requests whose Origin header is not in the
// configured allowlist. Needed on cookie-authenticated mutating routes
// because SameSite=None (required for cross-domain frontend/backend
// deployments) otherwise lets any site trigger these requests with the
// browser auto-attaching the refresh cookie.
func RequireTrustedOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowedOrigins, origin) {
				response.Error(w, http.StatusForbidden, "invalid or missing origin")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
