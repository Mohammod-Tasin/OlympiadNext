package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"olympiadnext/internal/auth"
	authemail "olympiadnext/internal/auth/email"
	"olympiadnext/internal/auth/google"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
)

type AuthHandler struct {
	authService *auth.Service
	users       user.Repository
	cookies     cookieConfig
	log         *slog.Logger
}

func NewAuthHandler(authService *auth.Service, users user.Repository, cookieDomain string, cookieSecure bool, cookieSameSite string, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		users:       users,
		cookies:     newCookieConfig(cookieDomain, cookieSecure, cookieSameSite),
		log:         log,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := h.authService.Register(r.Context(), req.Email, req.Password); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, map[string]string{
		"message": "account created; check your email for a verification code",
	})
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

	u, err := h.users.FindByID(r.Context(), claims.UserID)
	if err != nil {
		h.handleAuthError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, dto.UserResponse{
		UserID:             u.ID,
		Email:              u.Email,
		FullName:           u.FullName,
		EmailVerified:      u.EmailVerified,
		InstitutionName:    u.InstitutionName,
		Level:              u.Level,
		Medium:             u.Medium,
		ProfilePicture:     u.ProfilePicture,
		VerificationDoc:    u.VerificationDoc,
		VerificationStatus: string(u.VerificationStatus),
	})
}

// VerifyEmailOTP consumes the code emailed at registration and, on
// success, marks the account's email address as verified. It is
// unauthenticated: the caller has registered but cannot log in yet.
func (h *AuthHandler) VerifyEmailOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	email := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.OTP)
	if email == "" || code == "" {
		response.Error(w, http.StatusBadRequest, "email and otp are required")
		return
	}

	if err := h.authService.VerifyEmailOTP(r.Context(), email, code); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

// ResendEmailOTP issues a new verification code for an unverified
// account. The response is identical whether or not the address exists,
// so it cannot be used to enumerate registered emails.
func (h *AuthHandler) ResendEmailOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.ResendEmailOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		response.Error(w, http.StatusBadRequest, "email is required")
		return
	}

	if err := h.authService.ResendEmailOTP(r.Context(), email); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "if the account exists and is unverified, a new code has been sent",
	})
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
	case errors.Is(err, auth.ErrEmailNotVerified):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, auth.ErrInvalidOTP):
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
