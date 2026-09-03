package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/events"
	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
	"olympiadnext/internal/platform/storage"
)

// maxUploadBytes caps the entire upload request body at 100 MB, so
// high-resolution event images are accepted.
const maxUploadBytes = 100 << 20

// allowedImageExts is the set of extensions the upload endpoint accepts.
var allowedImageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

type EventHandler struct {
	events  *events.Service
	storage *storage.LocalStorage
	log     *slog.Logger
}

func NewEventHandler(eventService *events.Service, fileStorage *storage.LocalStorage, log *slog.Logger) *EventHandler {
	return &EventHandler{events: eventService, storage: fileStorage, log: log}
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

	if !allowedImageExts[strings.ToLower(filepath.Ext(header.Filename))] {
		response.Error(w, http.StatusBadRequest, "unsupported file type; allowed: jpg, jpeg, png, webp, gif")
		return
	}

	// Sniff the actual bytes so a renamed non-image is rejected too.
	sniff := make([]byte, 512)
	n, _ := io.ReadFull(file, sniff)
	if !strings.HasPrefix(http.DetectContentType(sniff[:n]), "image/") {
		response.Error(w, http.StatusBadRequest, "uploaded file is not a valid image")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		h.log.Error("event image upload: seek failed", "error", err)
		response.Error(w, http.StatusInternalServerError, "could not process file")
		return
	}

	url, err := h.storage.Save(header.Filename, file)
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
		Title:       req.Title,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		EventDate:   eventDate,
		IsActive:    req.IsActive,
	}, true
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
