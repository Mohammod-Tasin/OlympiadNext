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

// maxProfileFieldLength caps full_name and institution_name, matching the
// VARCHAR(255) columns they're stored in.
const maxProfileFieldLength = 255

// allowedLevels and allowedMediums are the fixed onboarding options the
// frontend presents; any other value is rejected rather than silently
// stored.
var allowedLevels = map[string]bool{
	"Junior":           true,
	"Secondary":        true,
	"Higher Secondary": true,
}

var allowedMediums = map[string]bool{
	"Bangla":  true,
	"English": true,
}

// validateProfileFields trims full_name, institution_name, level, and
// medium and enforces the mandatory-onboarding rules shared by Register
// and UpdateAcademicProfile: none may be empty, full_name/institution_name
// must fit their DB column, and level/medium must be one of the fixed
// allowed values. errMsg is empty when validation passes.
func validateProfileFields(fullName, institution, level, medium string) (trimmedFullName, trimmedInstitution, trimmedLevel, trimmedMedium, errMsg string) {
	trimmedFullName = strings.TrimSpace(fullName)
	trimmedInstitution = strings.TrimSpace(institution)
	trimmedLevel = strings.TrimSpace(level)
	trimmedMedium = strings.TrimSpace(medium)

	if trimmedFullName == "" || trimmedInstitution == "" || trimmedLevel == "" || trimmedMedium == "" {
		return "", "", "", "", "full_name, institution_name, level, and medium are required"
	}
	if len(trimmedFullName) > maxProfileFieldLength {
		return "", "", "", "", "full_name must be 255 characters or fewer"
	}
	if len(trimmedInstitution) > maxProfileFieldLength {
		return "", "", "", "", "institution_name must be 255 characters or fewer"
	}
	if !allowedLevels[trimmedLevel] {
		return "", "", "", "", "level must be one of: Junior, Secondary, Higher Secondary"
	}
	if !allowedMediums[trimmedMedium] {
		return "", "", "", "", "medium must be one of: Bangla, English"
	}
	return trimmedFullName, trimmedInstitution, trimmedLevel, trimmedMedium, ""
}

func NewAuthHandler(authService *auth.Service, users user.Repository, cookieDomain string, cookieSecure bool, cookieSameSite string, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		users:       users,
		cookies:     newCookieConfig(cookieDomain, cookieSecure, cookieSameSite),
		log:         log,
	}
}

// Register creates an unverified account and mails it an OTP. It issues
// no session on purpose — the client must call VerifyEmailOTP and then
// Login, so an unconfirmed address never gets tokens.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	fullName, institution, level, medium, errMsg := validateProfileFields(req.FullName, req.InstitutionName, req.Level, req.Medium)
	if errMsg != "" {
		response.Error(w, http.StatusBadRequest, errMsg)
		return
	}

	if err := h.authService.Register(r.Context(), req.Email, req.Password, fullName, institution, level, medium); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, map[string]string{
		"message": "account created, verification code sent to your email",
	})
}

// SendEmailOTP re-mails a verification code. The caller is
// unauthenticated, so the response is identical whether or not the
// address exists — otherwise this endpoint would enumerate accounts.
func (h *AuthHandler) SendEmailOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.SendEmailOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	emailAddr := strings.TrimSpace(req.Email)
	if emailAddr == "" {
		response.Error(w, http.StatusBadRequest, "email is required")
		return
	}

	err := h.authService.SendEmailOTP(r.Context(), emailAddr)
	switch {
	case err == nil, errors.Is(err, user.ErrNotFound), errors.Is(err, auth.ErrAlreadyVerified):
	default:
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "if the account exists and is unverified, a code has been sent",
	})
}

// VerifyEmailOTP consumes the code and marks the address verified.
func (h *AuthHandler) VerifyEmailOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	emailAddr := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.OTP)
	if emailAddr == "" || code == "" {
		response.Error(w, http.StatusBadRequest, "email and otp are required")
		return
	}

	if err := h.authService.VerifyEmailOTP(r.Context(), emailAddr, code); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"message": "email verified"})
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
		UserID:          u.ID,
		Email:           u.Email,
		FullName:        u.FullName,
		EmailVerified:   u.EmailVerified,
		InstitutionName: u.InstitutionName,
		Level:           u.Level,
		Medium:          u.Medium,
	})
}

// UpdateAcademicProfile sets the caller's full name, institution, academic
// level, and medium of instruction.
func (h *AuthHandler) UpdateAcademicProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.UpdateAcademicProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	fullName, institution, level, medium, errMsg := validateProfileFields(req.FullName, req.InstitutionName, req.Level, req.Medium)
	if errMsg != "" {
		response.Error(w, http.StatusBadRequest, errMsg)
		return
	}

	if err := h.users.UpdateAcademicProfile(r.Context(), claims.UserID, fullName, institution, level, medium); err != nil {
		h.handleAuthError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"message": "profile updated"})
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
		errors.Is(err, authemail.ErrWeakPassword),
		errors.Is(err, auth.ErrInvalidOTP),
		errors.Is(err, auth.ErrAlreadyVerified):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, user.ErrEmailTaken):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrGoogleOnlyAccount),
		errors.Is(err, auth.ErrSessionExpired):
		response.Error(w, http.StatusUnauthorized, err.Error())
	// 403 rather than 401: the password was right, the account just isn't
	// verified yet, so the client should route to the OTP screen instead
	// of back to the login form.
	case errors.Is(err, auth.ErrEmailNotVerified):
		response.Error(w, http.StatusForbidden, err.Error())
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
