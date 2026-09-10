package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/importantdates"
	"olympiadnext/internal/domain/importantdate"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
)

// importantDateLayout is the wire format for event_date on both the
// request body and every response — a calendar date, no time-of-day.
const importantDateLayout = "2006-01-02"

type ImportantDateHandler struct {
	dates *importantdates.Service
	log   *slog.Logger
}

func NewImportantDateHandler(dateService *importantdates.Service, log *slog.Logger) *ImportantDateHandler {
	return &ImportantDateHandler{dates: dateService, log: log}
}

// ListPublic handles GET /api/client/important-dates: the active rows
// only, ordered chronologically. Public, no authentication.
func (h *ImportantDateHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	list, err := h.dates.ListActive(r.Context())
	if err != nil {
		h.handleImportantDateError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toImportantDateListResponse(list))
}

// List handles GET /api/admin/important-dates: every row, active or not.
// Admin-gated by middleware.
func (h *ImportantDateHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.dates.ListAll(r.Context())
	if err != nil {
		h.handleImportantDateError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toImportantDateListResponse(list))
}

// Create handles POST /api/admin/important-dates. Admin-gated by middleware.
func (h *ImportantDateHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeImportantDateInput(w, r)
	if !ok {
		return
	}

	d, err := h.dates.Create(r.Context(), in)
	if err != nil {
		h.handleImportantDateError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toImportantDateResponse(d))
}

// Update handles PUT /api/admin/important-dates/{id}. Admin-gated by middleware.
func (h *ImportantDateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "important date id is required")
		return
	}

	in, ok := decodeImportantDateInput(w, r)
	if !ok {
		return
	}

	d, err := h.dates.Update(r.Context(), id, in)
	if err != nil {
		h.handleImportantDateError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toImportantDateResponse(d))
}

// Delete handles DELETE /api/admin/important-dates/{id}. Admin-gated by middleware.
func (h *ImportantDateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "important date id is required")
		return
	}

	if err := h.dates.Delete(r.Context(), id); err != nil {
		h.handleImportantDateError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"id": id, "deleted": "true"})
}

func (h *ImportantDateHandler) handleImportantDateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, importantdate.ErrNotFound):
		response.Error(w, http.StatusNotFound, "important date not found")
	case errors.Is(err, importantdates.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("important date handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// decodeImportantDateInput decodes the JSON body and strictly parses
// event_date as a "2006-01-02" calendar date. It writes the 400 response
// itself and returns ok=false on any failure.
func decodeImportantDateInput(w http.ResponseWriter, r *http.Request) (importantdates.Input, bool) {
	var req dto.ImportantDateRequest
	if !decodeJSON(w, r, &req) {
		return importantdates.Input{}, false
	}

	eventDate, err := time.Parse(importantDateLayout, strings.TrimSpace(req.EventDate))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "event_date must be a date in YYYY-MM-DD format, e.g. 2026-01-02")
		return importantdates.Input{}, false
	}

	return importantdates.Input{
		EventDate:    eventDate,
		Title:        req.Title,
		DetailsEN:    req.DetailsEN,
		DetailsBN:    req.DetailsBN,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.ActiveOrDefault(),
	}, true
}

func toImportantDateResponse(d *importantdate.ImportantDate) dto.ImportantDateResponse {
	return dto.ImportantDateResponse{
		ID:           d.ID,
		EventDate:    d.EventDate.Format(importantDateLayout),
		Title:        d.Title,
		DetailsEN:    d.DetailsEN,
		DetailsBN:    d.DetailsBN,
		DisplayOrder: d.DisplayOrder,
		IsActive:     d.IsActive,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

func toImportantDateListResponse(list []*importantdate.ImportantDate) dto.ImportantDateListResponse {
	out := make([]dto.ImportantDateResponse, 0, len(list))
	for _, d := range list {
		out = append(out, toImportantDateResponse(d))
	}
	return dto.ImportantDateListResponse{ImportantDates: out, Count: len(out)}
}
