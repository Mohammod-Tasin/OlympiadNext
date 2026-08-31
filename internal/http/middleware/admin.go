package middleware

import (
	"net/http"

	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/response"
)

// RequireAdmin gates a route to admin accounts. It must be chained after
// RequireAccessToken, which puts the validated access claims on the
// request context; the role itself is read fresh from the database so a
// demotion takes effect immediately rather than at next token refresh.
func RequireAdmin(users user.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := AccessClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated")
				return
			}

			role, err := users.GetRole(r.Context(), claims.UserID)
			if err != nil {
				response.Error(w, http.StatusForbidden, "admin access required")
				return
			}
			if role != user.RoleAdmin {
				response.Error(w, http.StatusForbidden, "admin access required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
