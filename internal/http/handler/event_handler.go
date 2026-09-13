package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/events"
	"olympiadnext/internal/app/registrations"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
	"olympiadnext/internal/platform/storage"
)

// maxUploadBytes caps the entire upload request body at 100 MB, so
// high-resolution event images are accepted.
const maxUploadBytes = 100 << 20

type EventHandler struct {
	events        *events.Service
	registrations *registrations.Service
	storage       *storage.LocalStorage
	jwt           *jwt.Manager
	log           *slog.Logger
}

func NewEventHandler(eventService *events.Service, registrationService *registrations.Service, fileStorage *storage.LocalStorage, jwtManager *jwt.Manager, log *slog.Logger) *EventHandler {
	return &EventHandler{events: eventService, registrations: registrationService, storage: fileStorage, jwt: jwtManager, log: log}
}

// GetActiveEvent serves the client surface: the single event currently
// published, or 404 when nothing is active.
//
// The route is public, but when the request carries a valid access token
// the response also reports whether that student has already registered
// for the event (is_registered), so the frontend can hide the payment
// form. The token is parsed here rather than via RequireAccessToken
// because the route must still serve anonymous callers, and because a
// browser cannot attach X-Device-Fingerprint to every content fetch.
func (h *EventHandler) GetActiveEvent(w http.ResponseWriter, r *http.Request) {
	e, err := h.events.GetActiveEvent(r.Context())
	if err != nil {
		h.handleEventError(w, err)
		return
	}

	resp := toEventResponse(e)
	h.attachIsRegistered(r, &resp, e.ID)
	response.JSON(w, http.StatusOK, resp)
}

// GetByID handles GET /api/client/events/{id}: a single event by id,
// regardless of whether it is the platform's current active one. Same
// response shape and optional-auth is_registered behavior as
// GetActiveEvent.
func (h *EventHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	e, err := h.events.GetEvent(r.Context(), id)
	if err != nil {
		h.handleEventError(w, err)
		return
	}

	resp := toEventResponse(e)
	h.attachIsRegistered(r, &resp, e.ID)
	response.JSON(w, http.StatusOK, resp)
}

// ListPublic handles GET /api/client/events/all: every event, active or
// not, ordered active-first then by event_date descending (see
// EventRepository.ListAll). Fully public — unlike GetActiveEvent/GetByID
// it does not attempt the optional-auth is_registered lookup, so every row
// reports is_registered: false; a future events-listing page that needs
// per-row registration state would call GetByID for the event the caller
// picks.
func (h *EventHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	list, err := h.events.ListEvents(r.Context())
	if err != nil {
		h.handleEventError(w, err)
		return
	}

	out := make([]dto.EventResponse, 0, len(list))
	for _, e := range list {
		out = append(out, toEventResponse(e))
	}
	response.JSON(w, http.StatusOK, dto.EventListResponse{Events: out, Count: len(out)})
}

// attachIsRegistered fills resp.IsRegistered when the request carries a
// valid access token, shared by GetActiveEvent and GetByID. A failed
// check should not hide the event; it logs and leaves the flag false
// rather than 500 the whole page.
func (h *EventHandler) attachIsRegistered(r *http.Request, resp *dto.EventResponse, eventID string) {
	claims, err := h.jwt.ParseAccessToken(middleware.ExtractBearerToken(r.Header.Get("Authorization")))
	if err != nil {
		return
	}
	registered, err := h.registrations.IsRegistered(r.Context(), claims.UserID, eventID)
	if err != nil {
		h.log.Error("client event: is-registered check failed", "user_id", claims.UserID, "event_id", eventID, "error", err)
		return
	}
	resp.IsRegistered = registered
}

// Create handles POST /api/admin/events (admin-gated by middleware).
func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeEventInput(w, r)
	if !ok {
		return
	}

	e, err := h.events.CreateEvent(r.Context(), in)
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

	in, ok := decodeEventInput(w, r)
	if !ok {
		return
	}

	e, err := h.events.UpdateEvent(r.Context(), id, in)
	if err != nil {
		h.handleEventError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toEventResponse(e))
}

// Upload handles POST /api/admin/events/upload: a multipart/form-data
// request with a single "file" field. It stores the image under a random
// UUID name and returns {"image_url": "/uploads/<uuid>.<ext>"}.
func (h *EventHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid multipart form or file larger than 100MB")
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

	// Sniff the leading bytes so a renamed non-image is rejected too.
	sniff := make([]byte, 512)
	n, _ := io.ReadFull(file, sniff)
	if err := storage.ValidateImage(header.Filename, sniff[:n]); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		h.log.Error("event image upload: seek failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not process file")
		return
	}

	// Event images sit at the uploads root (subdir ""); they are public.
	url, err := h.storage.Save("", header.Filename, file)
	if err != nil {
		h.log.Error("event image upload: save failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not save file")
		return
	}

	h.log.Info("event image uploaded", "image_url", url, "size_bytes", header.Size)
	response.JSON(w, http.StatusCreated, dto.UploadResponse{ImageURL: url})
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

// decodeEventInput decodes the JSON body and strictly parses event_date
// as an RFC3339 / ISO-8601 timestamp. It writes the 400 response itself
// and returns ok=false on any failure.
func decodeEventInput(w http.ResponseWriter, r *http.Request) (events.Input, bool) {
	var req dto.EventRequest
	if !decodeJSON(w, r, &req) {
		return events.Input{}, false
	}

	eventDate, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EventDate))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "event_date must be an RFC3339 timestamp, e.g. 2026-01-02T15:04:05Z")
		return events.Input{}, false
	}

	return events.Input{
		Title:           req.Title,
		Description:     req.Description,
		ImageURL:        req.ImageURL,
		EventDate:       eventDate,
		IsActive:        req.IsActive,
		BkashNumber:     req.BkashNumber,
		NagadNumber:     req.NagadNumber,
		RegistrationFee: req.RegistrationFee,
	}, true
}

func toEventResponse(e *event.Event) dto.EventResponse {
	return dto.EventResponse{
		ID:              e.ID,
		Title:           e.Title,
		Description:     e.Description,
		ImageURL:        e.ImageURL,
		EventDate:       e.EventDate,
		IsActive:        e.IsActive,
		BkashNumber:     e.BkashNumber,
		NagadNumber:     e.NagadNumber,
		RegistrationFee: e.RegistrationFee,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}
