package middleware

import (
	"net/http"

	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/response"
)

// RequireCompleteProfile gates access behind a fully onboarded, verified
// account: email verified, full name/institution/level/medium all filled
// in, and an admin-approved verification status. It must run after
// RequireAccessToken, since it reads the caller's identity from the
// access-claims context value that middleware sets. It is not applied to
// /api/auth/* or /api/user/* routes so callers can still finish
// onboarding; it's meant for routes reached only once verification must
// be complete (e.g. exams).
func RequireCompleteProfile(users user.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := AccessClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated")
				return
			}

			u, err := users.FindByID(r.Context(), claims.UserID)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "unauthenticated")
				return
			}

			if !isProfileComplete(u) {
				response.Error(w, http.StatusForbidden, "profile incomplete")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isProfileComplete(u *user.User) bool {
	return u.EmailVerified &&
		u.VerificationStatus == user.VerificationVerified &&
		hasValue(u.FullName) &&
		hasValue(u.InstitutionName) &&
		hasValue(u.Level) &&
		hasValue(u.Medium)
}

func hasValue(s *string) bool {
	return s != nil && *s != ""
}
