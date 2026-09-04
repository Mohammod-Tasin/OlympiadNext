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
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
	"olympiadnext/internal/platform/storage"
)

// maxUserUploadBytes caps a single KYC upload. Proof documents and profile
// photos are small; 15 MB leaves headroom for a high-resolution scan
// without inviting abuse of the shared uploads volume.
const maxUserUploadBytes = 15 << 20

// userFileSubdir is the per-user folder (under the uploads root) that all
// KYC files live in: uploads/users/<userID>/<uuid>.<ext>. The owner id in
// the path is what the gated /uploads/users route authorises against.
const userFileSubdir = "users"

type UserHandler struct {
	users   user.Repository
	storage *storage.LocalStorage
	log     *slog.Logger
}

func NewUserHandler(users user.Repository, fileStorage *storage.LocalStorage, log *slog.Logger) *UserHandler {
	return &UserHandler{users: users, storage: fileStorage, log: log}
}

// UploadFile handles POST /api/user/upload-file: a multipart/form-data
// request with a single "file" field (a PDF or an image). It stores the
// file under the caller's own folder and returns {"url": "..."}. The URL
// is what the caller then submits as verification_doc / profile_picture.
func (h *UserHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
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
		h.log.Error("user upload: seek failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not process file")
		return
	}

	url, err := h.storage.Save(path.Join(userFileSubdir, claims.UserID), header.Filename, file)
	if err != nil {
		h.log.Error("user upload: save failed", "user_id", claims.UserID, "error", err)
		response.Error(w, http.StatusInternalServerError, "could not save file")
		return
	}

	h.log.Info("user file uploaded", "user_id", claims.UserID, "url", url, "size_bytes", header.Size)
	response.JSON(w, http.StatusCreated, dto.UploadFileResponse{URL: url})
}

// SubmitProfile handles PUT /api/user/profile: the onboarding submission.
// It validates the academic fields, requires a verification_doc the caller
// actually uploaded, and moves the account to 'pending' review.
func (h *UserHandler) SubmitProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.SubmitProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	fullName, institution, level, medium, errMsg := validateProfileFields(req.FullName, req.InstitutionName, req.Level, req.Medium)
	if errMsg != "" {
		response.Error(w, http.StatusBadRequest, errMsg)
		return
	}

	doc := strings.TrimSpace(req.VerificationDoc)
	if doc == "" {
		response.Error(w, http.StatusBadRequest, "verification_doc is required")
		return
	}
	if !isOwnedUserFileURL(doc, claims.UserID) {
		response.Error(w, http.StatusBadRequest, "verification_doc must be a file you uploaded")
		return
	}

	var picture *string
	if p := strings.TrimSpace(req.ProfilePicture); p != "" {
		if !isOwnedUserFileURL(p, claims.UserID) {
			response.Error(w, http.StatusBadRequest, "profile_picture must be a file you uploaded")
			return
		}
		picture = &p
	}

	err := h.users.SubmitOnboardingProfile(r.Context(), claims.UserID, user.OnboardingProfile{
		FullName:        fullName,
		InstitutionName: institution,
		Level:           level,
		Medium:          medium,
		VerificationDoc: doc,
		ProfilePicture:  picture,
	})
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.log.Error("user submit profile failed", "user_id", claims.UserID, "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message":             "profile submitted for verification",
		"verification_status": string(user.VerificationPending),
	})
}

// ServeUserFile handles GET /uploads/users/{userID}/{name}: a KYC file is
// an identity document, so it is served only to its owner or an admin.
func (h *UserHandler) ServeUserFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	ownerID := chi.URLParam(r, "userID")
	name := chi.URLParam(r, "name")

	if claims.UserID != ownerID {
		role, err := h.users.GetRole(r.Context(), claims.UserID)
		if err != nil || role != user.RoleAdmin {
			// 404, not 403: don't confirm the file exists to a stranger.
			response.Error(w, http.StatusNotFound, "file not found")
			return
		}
	}

	diskPath, err := h.storage.UserFilePath(ownerID, name)
	if err != nil {
		response.Error(w, http.StatusNotFound, "file not found")
		return
	}
	http.ServeFile(w, r, diskPath)
}

// isOwnedUserFileURL reports whether rawURL is a "/uploads/users/<userID>/<file>"
// path with exactly one trailing file segment — i.e. a file the caller
// uploaded to their own folder. It stops a user from attaching another
// account's document URL as their own.
func isOwnedUserFileURL(rawURL, userID string) bool {
	prefix := storage.URLPrefix + "/" + userFileSubdir + "/" + userID + "/"
	rest, ok := strings.CutPrefix(rawURL, prefix)
	return ok && rest != "" && !strings.Contains(rest, "/") && rest == path.Base(rest)
}
