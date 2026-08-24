package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"olympiadnext/internal/auth"
	authemail "olympiadnext/internal/auth/email"
	"olympiadnext/internal/auth/google"
	"olympiadnext/internal/domain/otp"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
)

type AuthHandler struct {
	authService *auth.Service
	cookies     cookieConfig
	log         *slog.Logger
}

func NewAuthHandler(authService *auth.Service, cookieDomain string, cookieSecure bool, cookieSameSite string, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		cookies:     newCookieConfig(cookieDomain, cookieSecure, cookieSameSite),
		log:         log,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	pair, err := h.authService.Register(r.Context(), req.Email, req.Password, req.DeviceFingerprint)
	if err != nil {
		h.handleAuthError(w, err)
		return
	}
	h.respondWithSession(w, pair)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	pair, err := h.authService.Login(r.Context(), req.Email, req.Password, req.DeviceFingerprint)
	if err != nil {
		h.handleAuthError(w, err)
		return
	}
	h.respondWithSession(w, pair)
}

func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req dto.GoogleLoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.IDToken == "" {
		response.Error(w, http.StatusBadRequest, "id_token is required")
		return
	}

	pair, err := h.authService.GoogleLogin(r.Context(), req.IDToken, req.DeviceFingerprint)
	if err != nil {
		h.handleAuthError(w, err)
		return
	}
	h.respondWithSession(w, pair)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil || cookie.Value == "" {
		response.Error(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	pair, err := h.authService.Refresh(r.Context(), cookie.Value)
	if err != nil {
		clearRefreshCookie(w, h.cookies)
		h.handleAuthError(w, err)
		return
	}
	h.respondWithSession(w, pair)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(RefreshCookieName); err == nil && cookie.Value != "" {
		if err := h.authService.Logout(r.Context(), cookie.Value); err != nil {
			h.log.Error("logout: revoke failed", "error", err)
		}
	}
	clearRefreshCookie(w, h.cookies)
	response.JSON(w, http.StatusNoContent, nil)
}

// Me returns the identity of the caller, proving the access-token
// middleware and downstream handlers agree on the authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"user_id": claims.UserID,
		"email":   claims.Email,
	})
}

// SendOTP issues a 6-digit code for the authenticated caller to verify
// their email or phone.
func (h *AuthHandler) SendOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.SendOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	targetType, ok := parseOTPTargetType(req.Type)
	if !ok {
		response.Error(w, http.StatusBadRequest, "type must be 'email' or 'phone'")
		return
	}

	if err := h.authService.SendOTP(r.Context(), claims.UserID, targetType); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"message": "otp sent"})
}

// VerifyOTP checks a submitted code and, on success, marks the
// corresponding email/phone verification flag on the caller's account.
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.VerifyOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	targetType, ok := parseOTPTargetType(req.Type)
	if !ok {
		response.Error(w, http.StatusBadRequest, "type must be 'email' or 'phone'")
		return
	}
	if req.Code == "" {
		response.Error(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.authService.VerifyOTP(r.Context(), claims.UserID, targetType, req.Code); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"message": "verified"})
}

func parseOTPTargetType(raw string) (otp.TargetType, bool) {
	switch otp.TargetType(raw) {
	case otp.TargetEmail, otp.TargetPhone:
		return otp.TargetType(raw), true
	default:
		return "", false
	}
}

func (h *AuthHandler) respondWithSession(w http.ResponseWriter, pair *auth.TokenPair) {
	setRefreshCookie(w, h.cookies, pair.RefreshToken, pair.RefreshTokenExpiresAt)
	response.JSON(w, http.StatusOK, dto.AuthResponse{
		AccessToken:          pair.AccessToken,
		AccessTokenExpiresAt: pair.AccessTokenExpiresAt,
	})
}

func (h *AuthHandler) handleAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authemail.ErrInvalidEmail),
		errors.Is(err, authemail.ErrWeakPassword):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, user.ErrEmailTaken):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrGoogleOnlyAccount),
		errors.Is(err, auth.ErrSessionExpired):
		response.Error(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, auth.ErrInvalidOTPTarget),
		errors.Is(err, auth.ErrInvalidOTP):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, google.ErrInvalidToken):
		response.Error(w, http.StatusUnauthorized, "invalid Google credential")
	default:
		h.log.Error("auth handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
