// Package events contains the application-layer orchestration for the
// admin-managed event content shown on the client frontend. It depends
// only on domain interfaces, never on concrete infrastructure.
//
// Authorization (admin vs. client) is enforced at the HTTP middleware
// layer, not here: the service assumes its caller has already been
// vetted and focuses solely on validation and persistence.
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"olympiadnext/internal/domain/event"
)

// ErrValidation is returned when the caller-supplied event data is
// incomplete or malformed. Callers should map it to a 400 response.
var ErrValidation = errors.New("events: invalid event data")

// Input is the mutable state of an event, supplied by an admin on both
// create and update.
type Input struct {
	Title       string
	Description string
	ImageURL    string
	EventDate   time.Time
	IsActive    bool
}

func (in Input) validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrValidation)
	}
	if in.EventDate.IsZero() {
		return fmt.Errorf("%w: event_date is required", ErrValidation)
	}
	return nil
}

type Service struct {
	events event.Repository
	log    *slog.Logger
}

func NewService(events event.Repository, log *slog.Logger) *Service {
	return &Service{events: events, log: log}
}

// CreateEvent persists a new event. Admin-only at the transport layer.
func (s *Service) CreateEvent(ctx context.Context, in Input) (*event.Event, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	e := &event.Event{
		Title:       strings.TrimSpace(in.Title),
		Description: in.Description,
		ImageURL:    in.ImageURL,
		EventDate:   in.EventDate,
		IsActive:    in.IsActive,
	}
	if err := s.events.Create(ctx, e); err != nil {
		return nil, err
	}

	s.log.Info("event created", "event_id", e.ID, "is_active", e.IsActive)
	return e, nil
}

// UpdateEvent replaces the mutable fields of an existing event. Returns
// event.ErrNotFound when the id does not exist. Admin-only at the
// transport layer.
func (s *Service) UpdateEvent(ctx context.Context, id string, in Input) (*event.Event, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	existing, err := s.events.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	existing.Title = strings.TrimSpace(in.Title)
	existing.Description = in.Description
	existing.ImageURL = in.ImageURL
	existing.EventDate = in.EventDate
	existing.IsActive = in.IsActive

	if err := s.events.Update(ctx, existing); err != nil {
		return nil, err
	}

	s.log.Info("event updated", "event_id", existing.ID, "is_active", existing.IsActive)
	return existing, nil
}

// GetActiveEvent returns the event currently published to the client
// surface, or event.ErrNotFound when none is active. Public.
func (s *Service) GetActiveEvent(ctx context.Context) (*event.Event, error) {
	return s.events.FindActive(ctx)
}
