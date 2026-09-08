package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/notify"
	"olympiadnext/internal/app/registrations"
	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
	"olympiadnext/internal/platform/storage"
)

// admitCardSubdir is the protected top-level folder admit-card PDFs live
// in: uploads/admit-cards/<userID>/<uuid>.pdf. The gated /uploads route
// authorises the owner id in the path exactly as it does for KYC files.
const admitCardSubdir = "admit-cards"

// maxAdmitCardBytes caps an admit-card PDF upload. Admit cards are a page
// or two; 15 MB matches the KYC upload ceiling.
const maxAdmitCardBytes = 15 << 20

// maxRegistrationListLimit caps GET /api/admin/registrations, mirroring
// maxUserListLimit on the KYC queue: the review queue is small, and a hard
// cap keeps a missing filter from dumping the whole table.
const maxRegistrationListLimit = 200

type RegistrationHandler struct {
	registrations *registrations.Service
	storage       *storage.LocalStorage
	notifier      *notify.Service
	log           *slog.Logger
}

func NewRegistrationHandler(service *registrations.Service, fileStorage *storage.LocalStorage, notifier *notify.Service, log *slog.Logger) *RegistrationHandler {
	return &RegistrationHandler{registrations: service, storage: fileStorage, notifier: notifier, log: log}
}

// Create handles POST /api/user/registrations: a student submits a
// bKash/Nagad transaction id for an event's registration fee. Auth-gated
// by RequireAccessToken.
func (h *RegistrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req dto.CreateRegistrationRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	reg, err := h.registrations.Register(r.Context(), claims.UserID, registrations.RegisterInput{
		EventID:       req.EventID,
		PaymentMethod: req.PaymentMethod,
		SenderNumber:  req.SenderNumber,
		TransactionID: req.TransactionID,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, dto.RegistrationResponse{
		ID:            reg.ID,
		EventID:       reg.EventID,
		PaymentMethod: string(reg.PaymentMethod),
		SenderNumber:  reg.SenderNumber,
		TransactionID: reg.TransactionID,
		Status:        string(reg.Status),
		CreatedAt:     reg.CreatedAt,
	})
}

// ListMine handles GET /api/user/registrations: the caller's own
// registration(s) and their status. Auth-gated by RequireAccessToken.
func (h *RegistrationHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	details, err := h.registrations.ListForUser(r.Context(), claims.UserID)
	if err != nil {
		h.log.Error("list own registrations failed", "user_id", claims.UserID, "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	out := make([]dto.RegistrationResponse, 0, len(details))
	for _, d := range details {
		out = append(out, dto.RegistrationResponse{
			ID:                  d.ID,
			EventID:             d.EventID,
			EventTitle:          d.EventTitle,
			PaymentMethod:       string(d.PaymentMethod),
			SenderNumber:        d.SenderNumber,
			TransactionID:       d.TransactionID,
			Status:              string(d.Status),
			CreatedAt:           d.CreatedAt,
			ReviewedAt:          d.ReviewedAt,
			AdmitCardURL:        d.AdmitCardURL,
			AdmitCardUploadedAt: d.AdmitCardUploadedAt,
		})
	}
	response.JSON(w, http.StatusOK, dto.RegistrationListResponse{Registrations: out, Count: len(out)})
}

// ListForReview handles GET /api/admin/registrations, optionally filtered
// by ?status=pending and/or ?event_id=<event UUID>. Admin-gated by middleware.
func (h *RegistrationHandler) ListForReview(w http.ResponseWriter, r *http.Request) {
	var status registration.Status
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		status = registration.Status(raw)
		if !status.Valid() {
			response.Error(w, http.StatusBadRequest, "status must be one of: pending, approved, rejected")
			return
		}
	}

	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	details, err := h.registrations.ListForReview(r.Context(), status, eventID, maxRegistrationListLimit)
	if err != nil {
		// A mistyped ?event_id= is a client error (404), not a 500;
		// handleError maps registrations.ErrEventNotFound for us.
		h.handleError(w, err)
		return
	}

	out := make([]dto.AdminRegistrationResponse, 0, len(details))
	for _, d := range details {
		out = append(out, dto.AdminRegistrationResponse{
			ID:            d.ID,
			StudentName:   d.StudentName,
			StudentEmail:  d.StudentEmail,
			EventID:       d.EventID,
			EventTitle:    d.EventTitle,
			PaymentMethod: string(d.PaymentMethod),
			SenderNumber:  d.SenderNumber,
			TransactionID: d.TransactionID,
			Status:        string(d.Status),
			CreatedAt:     d.CreatedAt,
			ReviewedAt:    d.ReviewedAt,
		})
	}
	response.JSON(w, http.StatusOK, dto.AdminRegistrationListResponse{Registrations: out, Count: len(out)})
}

// Review handles PUT /api/admin/registrations/{id}/review with a JSON body
// {"status": "approved" | "rejected"}. reviewed_by is taken from the
// authenticated admin, not the request body. Admin-gated by middleware.
func (h *RegistrationHandler) Review(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "registration id is required")
		return
	}

	var req dto.ReviewRegistrationRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	decision := registration.Status(strings.TrimSpace(req.Status))
	if err := h.registrations.Review(r.Context(), id, claims.UserID, decision); err != nil {
		h.handleError(w, err)
		return
	}

	h.log.Info("exam registration reviewed", "registration_id", id, "status", decision)
	response.JSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": string(decision),
	})
}

// Unreject handles PUT /api/admin/registrations/{id}/unreject. It has no
// request body: this is a fixed rejected -> pending correction, not a
// general status editor. The service also clears reviewed_by/reviewed_at.
func (h *RegistrationHandler) Unreject(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "registration id is required")
		return
	}

	if err := h.registrations.Unreject(r.Context(), id, claims.UserID); err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": string(registration.StatusPending),
	})
}

// UploadAdmitCard handles POST /api/admin/registrations/{id}/admit-card: a
// multipart/form-data request with a single "file" field holding a PDF.
// The registration must be 'approved' (409 otherwise). The PDF is stored
// under uploads/admit-cards/<studentUserID>/, only the path is persisted,
// and the student is then notified on their preferred channel.
// Admin-gated by middleware.
func (h *RegistrationHandler) UploadAdmitCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "registration id is required")
		return
	}

	reg, err := h.registrations.Get(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}
	// Fail before touching the multipart body / storage when the
	// registration is not approved. SetAdmitCard re-checks this in SQL, so
	// a concurrent status change is still handled safely.
	if reg.Status != registration.StatusApproved {
		response.Error(w, http.StatusConflict, "an admit card can only be issued for an approved registration")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAdmitCardBytes)
	if err := r.ParseMultipartForm(maxAdmitCardBytes); err != nil {
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

	// Sniff the leading bytes so a renamed non-PDF is rejected.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	if err := storage.ValidatePDF(header.Filename, head[:n]); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		h.log.Error("admit card upload: seek failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not process file")
		return
	}

	url, err := h.storage.Save(path.Join(admitCardSubdir, reg.UserID), header.Filename, file)
	if err != nil {
		h.log.Error("admit card upload: save failed", "registration_id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, "could not save file")
		return
	}

	eventTitle, err := h.registrations.AttachAdmitCard(r.Context(), id, url)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Best-effort: the admit card is stored and downloadable regardless of
	// whether the notification goes out.
	h.notifier.NotifyAdmitCardReady(r.Context(), reg.UserID, eventTitle)

	h.log.Info("admit card uploaded", "registration_id", id, "user_id", reg.UserID, "url", url)
	response.JSON(w, http.StatusCreated, map[string]string{
		"id":             id,
		"admit_card_url": url,
	})
}

func (h *RegistrationHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, registrations.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, registrations.ErrEventNotFound):
		response.Error(w, http.StatusNotFound, "event not found")
	case errors.Is(err, registrations.ErrEventNotActive):
		response.Error(w, http.StatusConflict, "this event is not open for registration")
	case errors.Is(err, registration.ErrDuplicateTransactionID):
		response.Error(w, http.StatusConflict, "this transaction id has already been submitted")
	case errors.Is(err, registration.ErrAlreadyRegistered):
		response.Error(w, http.StatusConflict, "you are already registered for this event")
	case errors.Is(err, registration.ErrNotFound):
		response.Error(w, http.StatusNotFound, "registration not found")
	case errors.Is(err, registration.ErrAlreadyReviewed):
		response.Error(w, http.StatusConflict, "this registration has already been reviewed")
	case errors.Is(err, registration.ErrInvalidUnrejectTransition):
		response.Error(w, http.StatusConflict, "only a rejected registration can be returned to pending")
	case errors.Is(err, registration.ErrNotApproved):
		response.Error(w, http.StatusConflict, "an admit card can only be issued for an approved registration")
	default:
		h.log.Error("registration handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
