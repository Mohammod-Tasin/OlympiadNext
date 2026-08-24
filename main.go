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

	"olympiadnext/internal/auth"
	"olympiadnext/internal/auth/google"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/config"
	"olympiadnext/internal/http/handler"
	"olympiadnext/internal/logger"
	"olympiadnext/internal/platform/db"
	"olympiadnext/internal/repository/postgres"
	"olympiadnext/internal/server"
)

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
	jwtManager := jwt.NewManager(cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	googleVerifier := google.NewVerifier(cfg.GoogleClientID)

	authService := auth.NewService(userRepo, refreshTokenRepo, deviceRepo, jwtManager, googleVerifier, log)
	authHandler := handler.NewAuthHandler(authService, cfg.CookieDomain, cfg.CookieSecure, cfg.CookieSameSite, log)

	router := server.NewRouter(authHandler, jwtManager, userRepo, cfg.AllowedOrigins, log)

	srv := &http.Server{
		Addr:         "0.0.0.0:" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
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
