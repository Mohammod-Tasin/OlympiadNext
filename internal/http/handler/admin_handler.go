package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
)

// maxUserListLimit caps GET /api/admin/users. The review queue is small in
// practice; a hard cap keeps a missing filter from dumping the whole table.
const maxUserListLimit = 200

type AdminHandler struct {
	users user.Repository
	log   *slog.Logger
}

func NewAdminHandler(users user.Repository, log *slog.Logger) *AdminHandler {
	return &AdminHandler{users: users, log: log}
}

// ListUsers handles GET /api/admin/users, optionally filtered by
// ?status=<verification status> (e.g. ?status=pending for the review
// queue). Admin-gated by middleware.
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	var status user.VerificationStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		status = user.VerificationStatus(raw)
		if !status.Valid() {
			response.Error(w, http.StatusBadRequest, "status must be one of: unverified, pending, verified, rejected")
			return
		}
	}

	users, err := h.users.ListUsers(r.Context(), status, maxUserListLimit)
	if err != nil {
		h.log.Error("admin list users failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	out := make([]dto.AdminUserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUserResponse(u))
	}
	response.JSON(w, http.StatusOK, dto.AdminUserListResponse{Users: out, Count: len(out)})
}

// VerifyUser handles PUT /api/admin/users/{id}/verify with a JSON body
// {"status": "verified" | "rejected"}. Admin-gated by middleware.
func (h *AdminHandler) VerifyUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req dto.VerifyUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	status := user.VerificationStatus(strings.TrimSpace(req.Status))
	if status != user.VerificationVerified && status != user.VerificationRejected {
		response.Error(w, http.StatusBadRequest, `status must be "verified" or "rejected"`)
		return
	}

	if err := h.users.SetVerificationStatus(r.Context(), id, status); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.log.Error("admin verify user failed", "user_id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.log.Info("user verification reviewed", "user_id", id, "status", status)
	response.JSON(w, http.StatusOK, map[string]string{
		"user_id":             id,
		"verification_status": string(status),
	})
}

func toAdminUserResponse(u *user.User) dto.AdminUserResponse {
	return dto.AdminUserResponse{
		UserID:             u.ID,
		Email:              u.Email,
		FullName:           u.FullName,
		Role:               string(u.Role),
		EmailVerified:      u.EmailVerified,
		InstitutionName:    u.InstitutionName,
		Level:              u.Level,
		Medium:             u.Medium,
		ProfilePicture:     u.ProfilePicture,
		VerificationDoc:    u.VerificationDoc,
		VerificationStatus: string(u.VerificationStatus),
		CreatedAt:          u.CreatedAt,
	}
}
