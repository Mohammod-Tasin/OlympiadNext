package handler

import (
	"net/http"
	"strings"
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

// newCookieConfig resolves the SameSite string into the http.SameSite
// constant and enforces the platform invariant that SameSite=None must be
// paired with Secure=true: Go's net/http silently omits the SameSite=None
// attribute from the Set-Cookie header whenever Secure is false (see
// net/http.Cookie.String), which makes browsers fall back to the
// SameSite=Lax default and drop the cookie on cross-site requests — exactly
// the case of a Vercel frontend calling a Render backend. Enforcing it here
// means a misconfigured or mistyped COOKIE_SECURE env var can never
// silently break cross-domain auth.
func newCookieConfig(domain string, secure bool, sameSite string) cookieConfig {
	resolved := ParseSameSite(sameSite)
	if resolved == http.SameSiteNoneMode {
		secure = true
	}
	return cookieConfig{Domain: domain, Secure: secure, SameSite: resolved}
}

func ParseSameSite(raw string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
