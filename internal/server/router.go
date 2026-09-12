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
	adminAuthHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	adminHandler *handler.AdminHandler,
	eventHandler *handler.EventHandler,
	noticeHandler *handler.NoticeHandler,
	importantDateHandler *handler.ImportantDateHandler,
	registrationHandler *handler.RegistrationHandler,
	notificationHandler *handler.NotificationHandler,
	roundHandler *handler.RoundHandler,
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

	mountUploads(r, uploadsDir, userHandler)

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

		// Admin console auth, parallel to the student routes above and
		// running the same service logic. Two things differ: these routes
		// set/read a distinct refresh cookie (admin_refresh_token, scoped to
		// path /api/auth/admin) so an admin and a student can be signed in
		// at once in one browser without one login evicting the other, and
		// login requires the admin role (403 otherwise, no token issued).
		// They inherit this block's per-IP rate limit and CORS, and carry
		// the same RequireTrustedOrigin guard as every other mutating
		// /api/auth route.
		r.Route("/admin", func(r chi.Router) {
			r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/login", adminAuthHandler.Login)
			r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/refresh", adminAuthHandler.Refresh)
			r.With(appmw.RequireTrustedOrigin(allowedOrigins)).Post("/logout", adminAuthHandler.Logout)
		})
	})

	// Authenticated student surface: KYC file uploads and the onboarding
	// profile submission.
	r.Route("/api/user", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(30, 10))
		r.Use(appmw.RequireAccessToken(jwtManager, users))

		r.Post("/upload-file", userHandler.UploadFile)
		r.Put("/profile", userHandler.SubmitProfile)

		// Manual exam-registration payment: submit a bKash/Nagad TrxID,
		// then poll your own registration status. Same rate-limit tier as
		// the rest of /api/user/*.
		r.Post("/registrations", registrationHandler.Create)
		r.Get("/registrations", registrationHandler.ListMine)

		// Transactional-notification preference: pick email or SMS, and
		// prove a phone number with an OTP. resend-otp shares this group's
		// per-IP rate limit, the same tier the email-OTP resend runs under.
		r.Put("/notification-preference", notificationHandler.SetPreference)
		r.Post("/notification-phone/verify-otp", notificationHandler.VerifyPhoneOTP)
		r.Post("/notification-phone/resend-otp", notificationHandler.ResendPhoneOTP)
	})

	// Client surface: public, read-only content consumed by the client
	// frontend. No authentication required.
	r.Route("/api/client", func(r chi.Router) {
		r.Use(appmw.RateLimitByIP(60, 20))
		r.Get("/events", eventHandler.GetActiveEvent)
		r.Get("/events/{id}", eventHandler.GetByID)
		r.Get("/events/{eventID}/rounds", roundHandler.ListPublic)
		r.Get("/notices", noticeHandler.ListPublic)
		r.Get("/important-dates", importantDateHandler.ListPublic)

		// The real entry gate for a round: a client-side countdown is
		// never trusted. This is the one /api/client route that requires
		// auth, so it opts in per-route rather than moving the whole
		// group behind RequireAccessToken.
		r.With(appmw.RequireAccessToken(jwtManager, users)).Post("/rounds/{roundID}/enter", roundHandler.EnterRound)
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

			r.Get("/{eventID}/rounds", roundHandler.ListAdmin)
			r.Post("/{eventID}/rounds", roundHandler.Create)
			r.Put("/{eventID}/rounds/{id}", roundHandler.Update)
			r.Delete("/{eventID}/rounds/{id}", roundHandler.Delete)
		})

		// Round status transitions and the post-round decision workflow.
		// Not nested under /events since a round is addressed by its own
		// id from here on.
		r.Route("/rounds", func(r chi.Router) {
			r.Post("/{id}/start", roundHandler.Start)
			r.Post("/{id}/end", roundHandler.End)
			r.Get("/{id}/candidates", roundHandler.Candidates)
			r.Put("/{id}/participants", roundHandler.SetParticipants)
		})

		r.Route("/notices", func(r chi.Router) {
			r.Get("/", noticeHandler.List)
			r.Post("/", noticeHandler.Create)
			r.Put("/{id}", noticeHandler.Update)
			r.Delete("/{id}", noticeHandler.Delete)
		})

		r.Route("/important-dates", func(r chi.Router) {
			r.Get("/", importantDateHandler.List)
			r.Post("/", importantDateHandler.Create)
			r.Put("/{id}", importantDateHandler.Update)
			r.Delete("/{id}", importantDateHandler.Delete)
		})

		r.Get("/users", adminHandler.ListUsers)
		r.Put("/users/{id}/verify", adminHandler.VerifyUser)
		r.Post("/users/{id}/admit-card", adminHandler.UploadAdmitCard)

		// Exam-registration payment review queue, mirroring the KYC queue
		// above: list by ?status=pending, then approve/reject each.
		r.Get("/registrations", registrationHandler.ListForReview)
		r.Put("/registrations/{id}/review", registrationHandler.Review)
		r.Put("/registrations/{id}/unreject", registrationHandler.Unreject)
		r.Post("/registrations/{id}/admit-card", registrationHandler.UploadAdmitCard)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}

// mountUploads wires the static file server for uploaded assets. Event
// images (at the uploads root) are public; student KYC files, which live
// under users/<ownerID>/, are identity documents. ServeUserFile
// authenticates the Bearer token and enforces owner-or-admin access
// itself (it is not behind RequireAccessToken — see its doc comment), and
// the public file server explicitly refuses anything under users/.
func mountUploads(r chi.Router, uploadsDir string, userHandler *handler.UserHandler) {
	// Gated subtrees: identity documents (KYC) and admit cards. Each is
	// served only to the owning user or an admin, and the public file
	// server refuses anything under these prefixes.
	gatedPrefixes := []string{"users/", "admit-cards/"}

	publicFiles := http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir)))

	r.Route("/uploads", func(r chi.Router) {
		r.Get("/users/{userID}/{name}", userHandler.ServeUserFile)
		r.Get("/admit-cards/{userID}/{name}", userHandler.ServeAdmitCard)

		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rel := strings.TrimPrefix(req.URL.Path, "/uploads/")
			for _, p := range gatedPrefixes {
				if rel == strings.TrimSuffix(p, "/") || strings.HasPrefix(rel, p) {
					http.NotFound(w, req)
					return
				}
			}
			publicFiles.ServeHTTP(w, req)
		}))
	})
}
