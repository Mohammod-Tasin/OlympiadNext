package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/rounds"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/round"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/middleware"
	"olympiadnext/internal/http/response"
)

type RoundHandler struct {
	rounds *rounds.Service
	jwt    *jwt.Manager
	log    *slog.Logger
}

func NewRoundHandler(roundService *rounds.Service, jwtManager *jwt.Manager, log *slog.Logger) *RoundHandler {
	return &RoundHandler{rounds: roundService, jwt: jwtManager, log: log}
}

// ListPublic handles GET /api/client/events/{eventID}/rounds: every round
// for the event, ordered by round_order. The route is public, but when
// the request carries a valid access token each round also reports the
// caller's your_status, mirroring EventHandler.GetActiveEvent's
// optional-auth pattern.
func (h *RoundHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if eventID == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	list, err := h.rounds.ListRoundsByEvent(r.Context(), eventID)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}

	var userID string
	if claims, err := h.jwt.ParseAccessToken(middleware.ExtractBearerToken(r.Header.Get("Authorization"))); err == nil {
		userID = claims.UserID
	}

	out := make([]dto.RoundResponse, 0, len(list))
	for _, rnd := range list {
		resp := toRoundResponse(rnd)
		if userID != "" {
			status, err := h.rounds.YourStatus(r.Context(), userID, rnd)
			if err != nil {
				h.log.Error("round your-status check failed", "user_id", userID, "round_id", rnd.ID, "error", err)
			} else {
				resp.YourStatus = &status
			}
		}
		out = append(out, resp)
	}
	response.JSON(w, http.StatusOK, dto.RoundListResponse{Rounds: out, Count: len(out)})
}

// ListAdmin handles GET /api/admin/events/{eventID}/rounds: every round
// for the event, ordered by round_order, with no your_status (that's
// student-specific — see ListPublic). Admin-gated by middleware.
func (h *RoundHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if eventID == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	list, err := h.rounds.ListRoundsByEvent(r.Context(), eventID)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}

	out := make([]dto.RoundResponse, 0, len(list))
	for _, rnd := range list {
		out = append(out, toRoundResponse(rnd))
	}
	response.JSON(w, http.StatusOK, dto.RoundListResponse{Rounds: out, Count: len(out)})
}

// EnterRound handles POST /api/client/rounds/{roundID}/enter. This is the
// real security gate — a client-side countdown is never trusted. It
// requires an access token (RequireAccessToken, applied in the router)
// and answers 200 {"allowed":true} or 403 {"reason":"..."}.
func (h *RoundHandler) EnterRound(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AccessClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	roundID := strings.TrimSpace(chi.URLParam(r, "roundID"))
	if roundID == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	allowed, reason, err := h.rounds.EnterRound(r.Context(), roundID, claims.UserID)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	if !allowed {
		response.JSON(w, http.StatusForbidden, map[string]string{"reason": reason})
		return
	}
	response.JSON(w, http.StatusOK, map[string]bool{"allowed": true})
}

// Create handles POST /api/admin/events/{eventID}/rounds. Admin-gated by
// middleware.
func (h *RoundHandler) Create(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	if eventID == "" {
		response.Error(w, http.StatusBadRequest, "event id is required")
		return
	}

	in, ok := decodeRoundInput(w, r)
	if !ok {
		return
	}

	rnd, err := h.rounds.CreateRound(r.Context(), eventID, in)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toRoundResponse(rnd))
}

// Update handles PUT /api/admin/events/{eventID}/rounds/{id}. Admin-gated
// by middleware.
func (h *RoundHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	in, ok := decodeRoundInput(w, r)
	if !ok {
		return
	}

	rnd, err := h.rounds.UpdateRound(r.Context(), id, in)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toRoundResponse(rnd))
}

// Delete handles DELETE /api/admin/events/{eventID}/rounds/{id}.
// Admin-gated by middleware.
func (h *RoundHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	if err := h.rounds.DeleteRound(r.Context(), id); err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"id": id, "deleted": "true"})
}

// Start handles POST /api/admin/rounds/{id}/start: the explicit
// upcoming->ongoing transition. Admin-gated by middleware.
func (h *RoundHandler) Start(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	rnd, err := h.rounds.StartRound(r.Context(), id)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toRoundResponse(rnd))
}

// End handles POST /api/admin/rounds/{id}/end: the explicit
// ongoing->ended transition. Admin-gated by middleware.
func (h *RoundHandler) End(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	rnd, err := h.rounds.EndRound(r.Context(), id)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toRoundResponse(rnd))
}

// Candidates handles GET /api/admin/rounds/{id}/candidates: the dynamic
// eligible list for this round's next decision, only available once the
// round has ended. Admin-gated by middleware.
func (h *RoundHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	list, err := h.rounds.GetCandidates(r.Context(), id)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}

	out := make([]dto.CandidateResponse, 0, len(list))
	for _, c := range list {
		out = append(out, dto.CandidateResponse{
			UserID:         c.UserID,
			FullName:       c.FullName,
			Email:          c.Email,
			ExistingStatus: c.ExistingStatus,
		})
	}
	response.JSON(w, http.StatusOK, dto.CandidateListResponse{Candidates: out, Count: len(out)})
}

// SetParticipants handles PUT /api/admin/rounds/{id}/participants: a bulk
// upsert of qualify/eliminate/winner decisions, only available once the
// round has ended. Admin-gated by middleware.
func (h *RoundHandler) SetParticipants(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "round id is required")
		return
	}

	var req []dto.ParticipantDecisionRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	decisions := make([]rounds.ParticipantDecision, 0, len(req))
	for _, d := range req {
		decisions = append(decisions, rounds.ParticipantDecision{
			UserID: d.UserID,
			Status: round.ParticipantStatus(strings.TrimSpace(d.Status)),
		})
	}

	count, err := h.rounds.SetParticipants(r.Context(), id, decisions)
	if err != nil {
		h.handleRoundError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"round_id": id, "updated": count})
}

func (h *RoundHandler) handleRoundError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, round.ErrNotFound), errors.Is(err, round.ErrParticipantNotFound):
		response.Error(w, http.StatusNotFound, "round not found")
	case errors.Is(err, round.ErrInvalidTransition):
		response.Error(w, http.StatusConflict, "invalid round status transition")
	case errors.Is(err, round.ErrNotEnded):
		response.Error(w, http.StatusConflict, "round has not ended yet")
	case errors.Is(err, round.ErrDuplicateOrder):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, rounds.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("round handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// decodeRoundInput decodes the JSON body and strictly parses start_at as
// an RFC3339 / ISO-8601 timestamp, mirroring decodeEventInput. It writes
// the 400 response itself and returns ok=false on any failure.
func decodeRoundInput(w http.ResponseWriter, r *http.Request) (rounds.Input, bool) {
	var req dto.RoundRequest
	if !decodeJSON(w, r, &req) {
		return rounds.Input{}, false
	}

	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartAt))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "start_at must be an RFC3339 timestamp, e.g. 2026-01-02T15:04:05Z")
		return rounds.Input{}, false
	}

	return rounds.Input{
		RoundOrder:      req.RoundOrder,
		RoundName:       req.RoundName,
		StartAt:         startAt,
		DurationMinutes: req.DurationMinutes,
	}, true
}

func toRoundResponse(rnd *round.Round) dto.RoundResponse {
	return dto.RoundResponse{
		ID:              rnd.ID,
		EventID:         rnd.EventID,
		RoundOrder:      rnd.RoundOrder,
		RoundName:       rnd.RoundName,
		StartAt:         rnd.StartAt,
		DurationMinutes: rnd.DurationMinutes,
		Status:          string(rnd.Status),
		CreatedAt:       rnd.CreatedAt,
		UpdatedAt:       rnd.UpdatedAt,
	}
}
