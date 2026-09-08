package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"olympiadnext/internal/app/notify"
	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
)

// NotificationHandler serves the authenticated student's notification
// preference: choosing email or SMS and proving a phone number with an
// OTP. All routes sit in the /api/user group and are access-token gated.
type NotificationHandler struct {
	notifier *notify.Service
	log      *slog.Logger
}

func NewNotificationHandler(notifier *notify.Service, log *slog.Logger) *NotificationHandler {
	return &NotificationHandler{notifier: notifier, log: log}
}

// SetPreference handles PUT /api/user/notification-preference with a JSON
// body {"method": "email"|"phone", "phone": "01XXXXXXXXX"}. Choosing
// "phone" sends an OTP and returns 202 — the active channel does not
// change until the code is verified.
func (h *NotificationHandler) SetPreference(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.NotificationPreferenceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	pending, err := h.notifier.SetPreference(r.Context(), claims.UserID, req.Method, req.Phone)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if pending {
		response.JSON(w, http.StatusAccepted, map[string]string{
			"message":             "verification code sent; verify it to switch notifications to SMS",
			"notification_method": string(user.NotificationEmail),
		})
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message":             "notification method updated",
		"notification_method": string(user.NotificationEmail),
	})
}

// VerifyPhoneOTP handles POST /api/user/notification-phone/verify-otp with
// a JSON body {"otp": "123456"}.
func (h *NotificationHandler) VerifyPhoneOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.VerifyNotificationPhoneOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.OTP) == "" {
		response.Error(w, http.StatusBadRequest, "otp is required")
		return
	}

	if err := h.notifier.VerifyPhoneOTP(r.Context(), claims.UserID, req.OTP); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message":             "phone verified; notifications will now be sent by SMS",
		"notification_method": string(user.NotificationPhone),
	})
}

// ResendPhoneOTP handles POST /api/user/notification-phone/resend-otp. It
// takes no body and re-issues a code to the number already on file. It
// shares the /api/user per-IP rate limit, the same tier the email-OTP
// resend runs under.
func (h *NotificationHandler) ResendPhoneOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	if err := h.notifier.ResendPhoneOTP(r.Context(), claims.UserID); err != nil {
		h.handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{
		"message": "a new verification code has been sent",
	})
}

func (h *NotificationHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notify.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, notify.ErrInvalidOTP):
		response.Error(w, http.StatusBadRequest, "invalid or expired code")
	case errors.Is(err, notify.ErrNoPendingPhone):
		response.Error(w, http.StatusConflict, "no notification phone is awaiting verification")
	case errors.Is(err, user.ErrNotFound):
		response.Error(w, http.StatusNotFound, "user not found")
	default:
		h.log.Error("notification handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
