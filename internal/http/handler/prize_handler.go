package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/prizes"
	"olympiadnext/internal/domain/prize"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
)

type PrizeHandler struct {
	prizes *prizes.Service
	log    *slog.Logger
}

func NewPrizeHandler(prizeService *prizes.Service, log *slog.Logger) *PrizeHandler {
	return &PrizeHandler{prizes: prizeService, log: log}
}

// ListPublic handles GET /api/client/events/{eventID}/prizes: every prize
// tier for the event, ordered by rank_from. Public, no authentication.
func (h *PrizeHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	h.list(w, r)
}

// List handles GET /api/admin/events/{eventID}/prizes: the same listing
// as ListPublic — a prize tier has no draft/active state to distinguish
// the two surfaces. Admin-gated by middleware.
func (h *PrizeHandler) List(w http.ResponseWriter, r *http.Request) {
	h.list(w, r)
}

func (h *PrizeHandler) list(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if eventID == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	list, err := h.prizes.ListByEvent(r.Context(), eventID)
	if err != nil {
		h.handlePrizeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toPrizeListResponse(list))
}

// Create handles POST /api/admin/events/{eventID}/prizes. Admin-gated by
// middleware.
func (h *PrizeHandler) Create(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if eventID == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	in, ok := decodePrizeInput(w, r)
	if !ok {
		return
	}

	p, err := h.prizes.Create(r.Context(), eventID, in)
	if err != nil {
		h.handlePrizeError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toPrizeResponse(p))
}

// Update handles PUT /api/admin/events/{eventID}/prizes/{id}. Admin-gated
// by middleware.
func (h *PrizeHandler) Update(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if eventID == "" || id == "" {
		response.Error(w, http.StatusBadRequest, "event id and prize id are required")
		return
	}

	in, ok := decodePrizeInput(w, r)
	if !ok {
		return
	}

	p, err := h.prizes.Update(r.Context(), eventID, id, in)
	if err != nil {
		h.handlePrizeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toPrizeResponse(p))
}

// Delete handles DELETE /api/admin/events/{eventID}/prizes/{id}.
// Admin-gated by middleware.
func (h *PrizeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "prize id is required")
		return
	}

	if err := h.prizes.Delete(r.Context(), id); err != nil {
		h.handlePrizeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"id": id, "deleted": "true"})
}

func (h *PrizeHandler) handlePrizeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, prize.ErrNotFound):
		response.Error(w, http.StatusNotFound, "prize not found")
	case errors.Is(err, prizes.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("prize handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// decodePrizeInput decodes the JSON body. It writes the 400 response
// itself and returns ok=false on any failure.
func decodePrizeInput(w http.ResponseWriter, r *http.Request) (prizes.Input, bool) {
	var req dto.PrizeRequest
	if !decodeJSON(w, r, &req) {
		return prizes.Input{}, false
	}

	return prizes.Input{
		RankFrom:         req.RankFrom,
		RankTo:           req.RankTo,
		PrizeName:        req.PrizeName,
		PrizeDescription: req.PrizeDescription,
	}, true
}

func toPrizeResponse(p *prize.Prize) dto.PrizeResponse {
	return dto.PrizeResponse{
		ID:               p.ID,
		EventID:          p.EventID,
		RankFrom:         p.RankFrom,
		RankTo:           p.RankTo,
		PrizeName:        p.PrizeName,
		PrizeDescription: p.PrizeDescription,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func toPrizeListResponse(list []*prize.Prize) dto.PrizeListResponse {
	out := make([]dto.PrizeResponse, 0, len(list))
	for _, p := range list {
		out = append(out, toPrizeResponse(p))
	}
	return dto.PrizeListResponse{Prizes: out, Count: len(out)}
}
