package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/registrations"
	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
)

// maxRegistrationListLimit caps GET /api/admin/registrations, mirroring
// maxUserListLimit on the KYC queue: the review queue is small, and a hard
// cap keeps a missing filter from dumping the whole table.
const maxRegistrationListLimit = 200

type RegistrationHandler struct {
	registrations *registrations.Service
	log           *slog.Logger
}

func NewRegistrationHandler(service *registrations.Service, log *slog.Logger) *RegistrationHandler {
	return &RegistrationHandler{registrations: service, log: log}
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
			ID:            d.ID,
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
	response.JSON(w, http.StatusOK, dto.RegistrationListResponse{Registrations: out, Count: len(out)})
}

// ListForReview handles GET /api/admin/registrations, optionally filtered
// by ?status=pending for the review queue. Admin-gated by middleware.
func (h *RegistrationHandler) ListForReview(w http.ResponseWriter, r *http.Request) {
	var status registration.Status
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		status = registration.Status(raw)
		if !status.Valid() {
			response.Error(w, http.StatusBadRequest, "status must be one of: pending, approved, rejected")
			return
		}
	}

	details, err := h.registrations.ListForReview(r.Context(), status, maxRegistrationListLimit)
	if err != nil {
		h.log.Error("admin list registrations failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
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
	default:
		h.log.Error("registration handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
