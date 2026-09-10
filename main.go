package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"olympiadnext/internal/app/events"
	"olympiadnext/internal/app/importantdates"
	"olympiadnext/internal/app/notices"
	"olympiadnext/internal/app/notify"
	"olympiadnext/internal/app/registrations"
	"olympiadnext/internal/auth"
	"olympiadnext/internal/auth/google"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/config"
	"olympiadnext/internal/http/handler"
	"olympiadnext/internal/logger"
	"olympiadnext/internal/platform/db"
	"olympiadnext/internal/platform/email"
	platformsms "olympiadnext/internal/platform/sms"
	"olympiadnext/internal/platform/storage"
	"olympiadnext/internal/repository/postgres"
	"olympiadnext/internal/server"
)

// uploadsDir is the project-root folder where uploaded assets live —
// admin event images at the root, student KYC files under users/<id>/ —
// and from which the /uploads/* routes serve them.
const uploadsDir = "uploads"

func main() {
	_ = godotenv.Load() // optional; env vars set by the platform take precedence in prod

	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup: config error", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Env)

	conn, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Error("startup: db connect failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := db.Migrate(conn); err != nil {
		log.Error("startup: migration failed", "error", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	userRepo := postgres.NewUserRepository(conn)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(conn)
	deviceRepo := postgres.NewDeviceRepository(conn)
	eventRepo := postgres.NewEventRepository(conn)
	noticeRepo := postgres.NewNoticeRepository(conn)
	importantDateRepo := postgres.NewImportantDateRepository(conn)
	registrationRepo := postgres.NewRegistrationRepository(conn)
	emailSender := email.NewSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, log)
	// SMS is optional: an unset API key does not stop startup — the client
	// returns sms.ErrNotConfigured on the first send instead.
	smsSender := platformsms.NewBulkSMSBDClient(cfg.BulkSMSBDAPIKey, cfg.BulkSMSBDSenderID, log)
	jwtManager := jwt.NewManager(cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	googleVerifier := google.NewVerifier(cfg.GoogleClientID)

	authService := auth.NewService(userRepo, refreshTokenRepo, deviceRepo, emailSender, jwtManager, googleVerifier, log)
	authHandler := handler.NewAuthHandler(authService, userRepo, cfg.CookieDomain, cfg.CookieSecure, cfg.CookieSameSite, log)
	adminAuthHandler := handler.NewAdminAuthHandler(authService, userRepo, cfg.CookieDomain, cfg.CookieSecure, cfg.CookieSameSite, log)

	fileStorage, err := storage.NewLocalStorage(uploadsDir)
	if err != nil {
		log.Error("startup: uploads dir init failed", "error", err)
		os.Exit(1)
	}

	userHandler := handler.NewUserHandler(userRepo, fileStorage, jwtManager, log)
	adminHandler := handler.NewAdminHandler(userRepo, log)

	eventService := events.NewService(eventRepo, log)
	registrationService := registrations.NewService(registrationRepo, eventRepo, log)
	eventHandler := handler.NewEventHandler(eventService, registrationService, fileStorage, jwtManager, log)

	noticeService := notices.NewService(noticeRepo, log)
	noticeHandler := handler.NewNoticeHandler(noticeService, log)

	importantDateService := importantdates.NewService(importantDateRepo, log)
	importantDateHandler := handler.NewImportantDateHandler(importantDateService, log)

	notifyService := notify.NewService(userRepo, emailSender, smsSender, log)
	notificationHandler := handler.NewNotificationHandler(notifyService, log)

	registrationHandler := handler.NewRegistrationHandler(registrationService, fileStorage, notifyService, log)

	router := server.NewRouter(authHandler, adminAuthHandler, userHandler, adminHandler, eventHandler, noticeHandler, importantDateHandler, registrationHandler, notificationHandler, jwtManager, userRepo, cfg.AllowedOrigins, fileStorage.Dir(), log)

	srv := &http.Server{
		Addr:    "0.0.0.0:" + cfg.Port,
		Handler: router,
		// 5-minute read/write windows so a 100 MB event-image upload on a
		// slow connection is not cut off mid-transfer.
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info("server starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server: listen failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info("server shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server: graceful shutdown failed", "error", err)
	}
}
