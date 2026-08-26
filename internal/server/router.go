package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/handler"
	appmw "olympiadnext/internal/http/middleware"
)

func NewRouter(authHandler *handler.AuthHandler, jwtManager *jwt.Manager, users user.Repository, allowedOrigins []string, log *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(appmw.Logging(log))
	r.Use(appmw.CORS(allowedOrigins))

	// Render's health checks hit GET/HEAD / on startup; without an
	// explicit handler here they 404, which Render logs as noise on
	// every deploy and periodic ping.
	healthCheck := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	r.Get("/", healthCheck)
	r.Head("/", healthCheck)

	r.Route("/api/auth", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(30, 10))

		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/register", authHandler.Register)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/login", authHandler.Login)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/google", authHandler.GoogleLogin)

		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/refresh", authHandler.Refresh)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/logout", authHandler.Logout)

		r.Group(func(r chi.Router) {
			r.Use(appmw.RequireAccessToken(jwtManager, users))
			r.Get("/me", authHandler.Me)
			r.Post("/send-otp", authHandler.SendOTP)
			r.Post("/verify-otp", authHandler.VerifyOTP)
			r.Post("/update-phone", authHandler.UpdatePhoneNumber)
			r.Put("/profile", authHandler.UpdateAcademicProfile)
		})
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}
