package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/notices"
	"olympiadnext/internal/domain/notice"
	"olympiadnext/internal/http/dto"
	"olympiadnext/internal/http/response"
)

type NoticeHandler struct {
	notices *notices.Service
	log     *slog.Logger
}

func NewNoticeHandler(noticeService *notices.Service, log *slog.Logger) *NoticeHandler {
	return &NoticeHandler{notices: noticeService, log: log}
}

// ListPublic handles GET /api/client/notices: the active notices only,
// ordered by display_order ascending. Public, no authentication.
func (h *NoticeHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	list, err := h.notices.ListActive(r.Context())
	if err != nil {
		h.handleNoticeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toNoticeListResponse(list))
}

// List handles GET /api/admin/notices: every notice, active or not.
// Admin-gated by middleware.
func (h *NoticeHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.notices.ListAll(r.Context())
	if err != nil {
		h.handleNoticeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toNoticeListResponse(list))
}

// Create handles POST /api/admin/notices. Admin-gated by middleware.
func (h *NoticeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.NoticeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	n, err := h.notices.Create(r.Context(), notices.Input{
		TextEN:       req.TextEN,
		TextBN:       req.TextBN,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.ActiveOrDefault(),
	})
	if err != nil {
		h.handleNoticeError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toNoticeResponse(n))
}

// Update handles PUT /api/admin/notices/{id}. Admin-gated by middleware.
func (h *NoticeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "notice id is required")
		return
	}

	var req dto.NoticeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	n, err := h.notices.Update(r.Context(), id, notices.Input{
		TextEN:       req.TextEN,
		TextBN:       req.TextBN,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.ActiveOrDefault(),
	})
	if err != nil {
		h.handleNoticeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toNoticeResponse(n))
}

// Delete handles DELETE /api/admin/notices/{id}. Admin-gated by middleware.
func (h *NoticeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "notice id is required")
		return
	}

	if err := h.notices.Delete(r.Context(), id); err != nil {
		h.handleNoticeError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"id": id, "deleted": "true"})
}

func (h *NoticeHandler) handleNoticeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notice.ErrNotFound):
		response.Error(w, http.StatusNotFound, "notice not found")
	case errors.Is(err, notices.ErrValidation):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("notice handler: unexpected error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func toNoticeResponse(n *notice.Notice) dto.NoticeResponse {
	return dto.NoticeResponse{
		ID:           n.ID,
		TextEN:       n.TextEN,
		TextBN:       n.TextBN,
		DisplayOrder: n.DisplayOrder,
		IsActive:     n.IsActive,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
	}
}

func toNoticeListResponse(list []*notice.Notice) dto.NoticeListResponse {
	out := make([]dto.NoticeResponse, 0, len(list))
	for _, n := range list {
		out = append(out, toNoticeResponse(n))
	}
	return dto.NoticeListResponse{Notices: out, Count: len(out)}
}
