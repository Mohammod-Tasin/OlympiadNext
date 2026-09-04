package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/handler"
	appmw "olympiadnext/internal/http/middleware"
)

func NewRouter(
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	adminHandler *handler.AdminHandler,
	eventHandler *handler.EventHandler,
	jwtManager *jwt.Manager,
	users user.Repository,
	allowedOrigins []string,
	uploadsDir string,
	log *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(appmw.Logging(log))
	r.Use(appmw.CORS(allowedOrigins))

	mountUploads(r, uploadsDir, userHandler, jwtManager, users)

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
		})
	})

	// Authenticated student surface: KYC file uploads and the onboarding
	// profile submission.
	r.Route("/api/user", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(30, 10))
		r.Use(appmw.RequireAccessToken(jwtManager, users))

		r.Post("/upload-file", userHandler.UploadFile)
		r.Put("/profile", userHandler.SubmitProfile)
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

		r.Get("/users", adminHandler.ListUsers)
		r.Put("/users/{id}/verify", adminHandler.VerifyUser)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}

// mountUploads wires the static file server for uploaded assets. Event
// images (at the uploads root) are public; student KYC files, which live
// under users/<ownerID>/, are identity documents and are served only
// through the authenticated ServeUserFile handler — the public file
// server explicitly refuses anything under users/.
func mountUploads(r chi.Router, uploadsDir string, userHandler *handler.UserHandler, jwtManager *jwt.Manager, users user.Repository) {
	const userPrefix = "users/"

	publicFiles := http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir)))

	r.Route("/uploads", func(r chi.Router) {
		r.With(appmw.RequireAccessToken(jwtManager, users)).
			Get("/users/{userID}/{name}", userHandler.ServeUserFile)

		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rel := strings.TrimPrefix(req.URL.Path, "/uploads/")
			if rel == strings.TrimSuffix(userPrefix, "/") || strings.HasPrefix(rel, userPrefix) {
				http.NotFound(w, req)
				return
			}
			publicFiles.ServeHTTP(w, req)
		}))
	})
}
