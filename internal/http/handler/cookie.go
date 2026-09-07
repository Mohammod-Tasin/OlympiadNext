package handler

import (
	"net/http"
	"strings"
	"time"
)

// Refresh-cookie identities for the two auth surfaces. The client
// (student) frontend and the admin console are deployed to different
// origins but call this one backend, so the browser stores the refresh
// cookie keyed only by (name, path) on the backend's domain. Giving admin
// sessions a distinct name *and* a path scoped to the admin auth subtree
// lets a student and an admin stay logged in side by side in one browser,
// instead of whichever login happened last silently evicting the other.
const (
	studentRefreshCookieName = "refresh_token"
	studentRefreshCookiePath = "/api/auth"

	adminRefreshCookieName = "admin_refresh_token"
	adminRefreshCookiePath = "/api/auth/admin"
)

func setRefreshCookie(w http.ResponseWriter, cfg cookieConfig, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    value,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

func clearRefreshCookie(w http.ResponseWriter, cfg cookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    "",
		Path:     cfg.Path,
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
// Name and Path distinguish the student and admin refresh cookies (see
// the constants above); the rest come from deployment env vars.
type cookieConfig struct {
	Name     string
	Path     string
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
func newCookieConfig(name, path, domain string, secure bool, sameSite string) cookieConfig {
	resolved := ParseSameSite(sameSite)
	if resolved == http.SameSiteNoneMode {
		secure = true
	}
	return cookieConfig{Name: name, Path: path, Domain: domain, Secure: secure, SameSite: resolved}
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
