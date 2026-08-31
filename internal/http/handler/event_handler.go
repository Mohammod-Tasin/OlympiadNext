package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/events"
	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
)

type EventHandler struct {
	events *events.Service
	log    *slog.Logger
}

func NewEventHandler(eventService *events.Service, log *slog.Logger) *EventHandler {
	return &EventHandler{events: eventService, log: log}
}

// GetActiveEvent serves the client surface: the single event currently
// published, or 404 when nothing is active.
func (h *EventHandler) GetActiveEvent(w http.ResponseWriter, r *http.Request) {
	e, err := h.events.GetActiveEvent(r.Context())
	if err != nil {
		h.handleEventError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toEventResponse(e))
}

// Create handles POST /api/admin/events (admin-gated by middleware).
func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.EventRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	e, err := h.events.CreateEvent(r.Context(), toEventInput(req))
	if err != nil {
		h.handleEventError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toEventResponse(e))
}

// Update handles PUT /api/admin/events/{eventID} (admin-gated by middleware).
func (h *EventHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	var req dto.EventRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	e, err := h.events.UpdateEvent(r.Context(), id, toEventInput(req))
	if err != nil {
		h.handleEventError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toEventResponse(e))
}

func (h *EventHandler) handleEventError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, event.ErrNotFound):
		response.Error(w, http.StatusNotFound, "event not found")
	case errors.Is(err, events.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("event handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func toEventInput(req dto.EventRequest) events.Input {
	return events.Input{
		Title:       req.Title,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		EventDate:   req.EventDate,
		IsActive:    req.IsActive,
	}
}

func toEventResponse(e *event.Event) dto.EventResponse {
	return dto.EventResponse{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		ImageURL:    e.ImageURL,
		EventDate:   e.EventDate,
		IsActive:    e.IsActive,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}
