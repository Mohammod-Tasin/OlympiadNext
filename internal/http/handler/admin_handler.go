package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/domain/user"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
	"olympiadnext/internal/platform/storage"
)

// maxUserListLimit caps GET /api/admin/users. The review queue is small in
// practice; a hard cap keeps a missing filter from dumping the whole table.
const maxUserListLimit = 200

type AdminHandler struct {
	users   user.Repository
	storage *storage.LocalStorage
	log     *slog.Logger
}

func NewAdminHandler(users user.Repository, fileStorage *storage.LocalStorage, log *slog.Logger) *AdminHandler {
	return &AdminHandler{users: users, storage: fileStorage, log: log}
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

// UploadAdmitCard handles POST /api/admin/users/{id}/admit-card: a
// multipart/form-data request with a single "file" field (PDF or image).
// The file is stored under uploads/users/<id>/, the same owner-or-admin
// gated folder KYC documents use, and users.admit_card_url is updated to
// its path. This is a general per-student admit card, independent of the
// per-registration admit card issued via
// POST /api/admin/registrations/{id}/admit-card. Admin-gated by middleware.
func (h *AdminHandler) UploadAdmitCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "user id is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUserUploadBytes)
	if err := r.ParseMultipartForm(maxUserUploadBytes); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid multipart form or file larger than 15MB")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "missing form field 'file'")
		return
	}
	defer file.Close()

	// Sniff the leading bytes so a renamed non-document is rejected.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	if err := storage.ValidateDocument(header.Filename, head[:n]); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		h.log.Error("admit card upload: seek failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not process file")
		return
	}

	url, err := h.storage.Save(path.Join(userFileSubdir, id), header.Filename, file)
	if err != nil {
		h.log.Error("admit card upload: save failed", "user_id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, "could not save file")
		return
	}

	if err := h.users.SetAdmitCardURL(r.Context(), id, url); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.log.Error("admin upload admit card failed", "user_id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.log.Info("admit card uploaded", "user_id", id, "url", url)
	response.JSON(w, http.StatusCreated, map[string]string{
		"user_id":        id,
		"admit_card_url": url,
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
