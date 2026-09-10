// Package importantdates contains the application-layer orchestration for
// the admin-managed "important dates" list shown on the client frontend.
// It depends only on domain interfaces, never on concrete infrastructure.
//
// Authorization (admin vs. client) is enforced at the HTTP middleware
// layer, not here: the service assumes its caller has already been
// vetted and focuses solely on validation and persistence.
package importantdates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"olympiadnext/internal/domain/importantdate"
)

// ErrValidation is returned when the caller-supplied data is incomplete
// or malformed. Callers should map it to a 400 response.
var ErrValidation = errors.New("importantdates: invalid important date data")

// Input is the mutable state of an important date, supplied by an admin
// on both create and update.
type Input struct {
	EventDate    time.Time
	Title        string
	DetailsEN    string
	DetailsBN    string
	DisplayOrder int
	IsActive     bool
}

func (in Input) validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrValidation)
	}
	if strings.TrimSpace(in.DetailsEN) == "" {
		return fmt.Errorf("%w: details_en is required", ErrValidation)
	}
	if strings.TrimSpace(in.DetailsBN) == "" {
		return fmt.Errorf("%w: details_bn is required", ErrValidation)
	}
	if in.DisplayOrder < 0 {
		return fmt.Errorf("%w: display_order cannot be negative", ErrValidation)
	}
	if in.EventDate.IsZero() {
		return fmt.Errorf("%w: event_date is required", ErrValidation)
	}
	return nil
}

type Service struct {
	dates importantdate.Repository
	log   *slog.Logger
}

func NewService(dates importantdate.Repository, log *slog.Logger) *Service {
	return &Service{dates: dates, log: log}
}

// Create persists a new important date. Admin-only at the transport layer.
func (s *Service) Create(ctx context.Context, in Input) (*importantdate.ImportantDate, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	d := &importantdate.ImportantDate{
		EventDate:    in.EventDate,
		Title:        strings.TrimSpace(in.Title),
		DetailsEN:    strings.TrimSpace(in.DetailsEN),
		DetailsBN:    strings.TrimSpace(in.DetailsBN),
		DisplayOrder: in.DisplayOrder,
		IsActive:     in.IsActive,
	}
	if err := s.dates.Create(ctx, d); err != nil {
		return nil, err
	}

	s.log.Info("important date created", "important_date_id", d.ID, "is_active", d.IsActive)
	return d, nil
}

// Update replaces the mutable fields of an existing important date.
// Returns importantdate.ErrNotFound when the id does not exist. Admin-only
// at the transport layer.
func (s *Service) Update(ctx context.Context, id string, in Input) (*importantdate.ImportantDate, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	d := &importantdate.ImportantDate{
		ID:           strings.TrimSpace(id),
		EventDate:    in.EventDate,
		Title:        strings.TrimSpace(in.Title),
		DetailsEN:    strings.TrimSpace(in.DetailsEN),
		DetailsBN:    strings.TrimSpace(in.DetailsBN),
		DisplayOrder: in.DisplayOrder,
		IsActive:     in.IsActive,
	}
	if err := s.dates.Update(ctx, d); err != nil {
		return nil, err
	}

	s.log.Info("important date updated", "important_date_id", d.ID, "is_active", d.IsActive)
	return d, nil
}

// Delete removes an important date. Returns importantdate.ErrNotFound when
// the id does not exist. Admin-only at the transport layer.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.dates.Delete(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	s.log.Info("important date deleted", "important_date_id", id)
	return nil
}

// ListAll returns every important date, active or not, for the admin
// console.
func (s *Service) ListAll(ctx context.Context) ([]*importantdate.ImportantDate, error) {
	return s.dates.ListAll(ctx)
}

// ListActive returns the active important dates in chronological order
// for the public list.
func (s *Service) ListActive(ctx context.Context) ([]*importantdate.ImportantDate, error) {
	return s.dates.ListActive(ctx)
}
