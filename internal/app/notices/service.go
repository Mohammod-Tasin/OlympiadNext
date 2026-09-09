// Package notices contains the application-layer orchestration for the
// admin-managed notice board shown on the client frontend. It depends
// only on domain interfaces, never on concrete infrastructure.
//
// Authorization (admin vs. client) is enforced at the HTTP middleware
// layer, not here: the service assumes its caller has already been
// vetted and focuses solely on validation and persistence.
package notices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"olympiadnext/internal/domain/notice"
)

// ErrValidation is returned when the caller-supplied notice data is
// incomplete or malformed. Callers should map it to a 400 response.
var ErrValidation = errors.New("notices: invalid notice data")

// Input is the mutable state of a notice, supplied by an admin on both
// create and update.
type Input struct {
	TextEN       string
	TextBN       string
	DisplayOrder int
	IsActive     bool
}

func (in Input) validate() error {
	if strings.TrimSpace(in.TextEN) == "" {
		return fmt.Errorf("%w: text_en is required", ErrValidation)
	}
	if strings.TrimSpace(in.TextBN) == "" {
		return fmt.Errorf("%w: text_bn is required", ErrValidation)
	}
	if in.DisplayOrder < 0 {
		return fmt.Errorf("%w: display_order cannot be negative", ErrValidation)
	}
	return nil
}

type Service struct {
	notices notice.Repository
	log     *slog.Logger
}

func NewService(notices notice.Repository, log *slog.Logger) *Service {
	return &Service{notices: notices, log: log}
}

// Create persists a new notice. Admin-only at the transport layer.
func (s *Service) Create(ctx context.Context, in Input) (*notice.Notice, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	n := &notice.Notice{
		TextEN:       strings.TrimSpace(in.TextEN),
		TextBN:       strings.TrimSpace(in.TextBN),
		DisplayOrder: in.DisplayOrder,
		IsActive:     in.IsActive,
	}
	if err := s.notices.Create(ctx, n); err != nil {
		return nil, err
	}

	s.log.Info("notice created", "notice_id", n.ID, "is_active", n.IsActive)
	return n, nil
}

// Update replaces the mutable fields of an existing notice. Returns
// notice.ErrNotFound when the id does not exist. Admin-only at the
// transport layer.
func (s *Service) Update(ctx context.Context, id string, in Input) (*notice.Notice, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	n := &notice.Notice{
		ID:           strings.TrimSpace(id),
		TextEN:       strings.TrimSpace(in.TextEN),
		TextBN:       strings.TrimSpace(in.TextBN),
		DisplayOrder: in.DisplayOrder,
		IsActive:     in.IsActive,
	}
	if err := s.notices.Update(ctx, n); err != nil {
		return nil, err
	}

	s.log.Info("notice updated", "notice_id", n.ID, "is_active", n.IsActive)
	return n, nil
}

// Delete removes a notice. Returns notice.ErrNotFound when the id does
// not exist. Admin-only at the transport layer.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.notices.Delete(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	s.log.Info("notice deleted", "notice_id", id)
	return nil
}

// ListAll returns every notice, active or not, for the admin console.
func (s *Service) ListAll(ctx context.Context) ([]*notice.Notice, error) {
	return s.notices.ListAll(ctx)
}

// ListActive returns the active notices in display order for the public
// board.
func (s *Service) ListActive(ctx context.Context) ([]*notice.Notice, error) {
	return s.notices.ListActive(ctx)
}
