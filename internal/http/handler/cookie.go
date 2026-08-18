package handler

import (
	"net/http"
	"time"
)

const RefreshCookieName = "refresh_token"

func setRefreshCookie(w http.ResponseWriter, cfg cookieConfig, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    value,
		Path:     "/api/auth",
		Domain:   cfg.Domain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

func clearRefreshCookie(w http.ResponseWriter, cfg cookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		Domain:   cfg.Domain,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

// cookieConfig is the minimal set of deployment-specific cookie
// attributes the handler needs, decoupled from the global app config.
type cookieConfig struct {
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

func ParseSameSite(raw string) http.SameSite {
	switch raw {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
