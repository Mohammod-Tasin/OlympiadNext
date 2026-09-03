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

func NewRouter(authHandler *handler.AuthHandler, eventHandler *handler.EventHandler, jwtManager *jwt.Manager, users user.Repository, allowedOrigins []string, uploadsDir string, log *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(appmw.Logging(log))
	r.Use(appmw.CORS(allowedOrigins))

	// Serve admin-uploaded event images. StripPrefix maps the public
	// "/uploads/*" path onto the on-disk uploads directory.
	fileServer := http.FileServer(http.Dir(uploadsDir))
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", fileServer))

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
		// Email verification is unauthenticated by necessity: a user who
		// has not verified yet is never issued an access token.
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/verify-email-otp", authHandler.VerifyEmailOTP)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/resend-email-otp", authHandler.ResendEmailOTP)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/login", authHandler.Login)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/google", authHandler.GoogleLogin)

		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/refresh", authHandler.Refresh)
		r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/logout", authHandler.Logout)

		r.Group(func(r chi.Router) {
			r.Use(appmw.RequireAccessToken(jwtManager, users))
			r.Get("/me", authHandler.Me)
			r.Put("/profile", authHandler.UpdateAcademicProfile)
		})
	})

	// Client surface: public, read-only content consumed by the client
	// frontend. No authentication required.
	r.Route("/api/client", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(60, 20))
		r.Get("/events", eventHandler.GetActiveEvent)
	})

	// Admin surface: every route requires a valid access token AND an
	// admin role. Kept as a wholly separate tree from the client routes.
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(60, 20))
		r.Use(appmw.RequireAccessToken(jwtManager, users))
		r.Use(appmw.RequireAdmin(users))

		r.Route("/events", func(r chi.Router) {
			r.Post("/", eventHandler.Create)
			r.Post("/upload", eventHandler.Upload)
			r.Put("/{eventID}", eventHandler.Update)
		})
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}
