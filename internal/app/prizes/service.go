// Package prizes contains the application-layer orchestration for an
// event's admin-configured prize tiers: admin CRUD plus the public
// listing the client frontend renders. It depends only on domain
// interfaces, never on concrete infrastructure.
//
// Authorization (admin vs. client) is enforced at the HTTP middleware
// layer, not here: the service assumes its caller has already been
// vetted and focuses solely on validation and persistence.
package prizes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"olympiadnext/internal/domain/prize"
)

// ErrValidation is returned when the caller-supplied prize data is
// incomplete, malformed, or overlaps an existing tier. Callers should map
// it to a 400 response.
var ErrValidation = errors.New("prizes: invalid prize data")

// Input is the mutable state of a prize tier, supplied by an admin on
// both create and update.
type Input struct {
	RankFrom         int
	RankTo           int
	PrizeName        string
	PrizeDescription *string
}

func (in Input) validate() error {
	if in.RankFrom <= 0 {
		return fmt.Errorf("%w: rank_from must be a positive integer", ErrValidation)
	}
	if in.RankTo <= 0 {
		return fmt.Errorf("%w: rank_to must be a positive integer", ErrValidation)
	}
	if in.RankFrom > in.RankTo {
		return fmt.Errorf("%w: rank_from must be less than or equal to rank_to", ErrValidation)
	}
	if strings.TrimSpace(in.PrizeName) == "" {
		return fmt.Errorf("%w: prize_name is required", ErrValidation)
	}
	return nil
}

// normalizedDescription trims the input and turns an empty result into
// nil, so a blank string is stored as SQL NULL rather than "".
func normalizedDescription(in *string) *string {
	if in == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*in)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

type Service struct {
	prizes prize.Repository
	log    *slog.Logger
}

func NewService(prizes prize.Repository, log *slog.Logger) *Service {
	return &Service{prizes: prizes, log: log}
}

// Create persists a new prize tier for an event. Admin-only at the
// transport layer.
func (s *Service) Create(ctx context.Context, eventID string, in Input) (*prize.Prize, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.checkNoOverlap(ctx, eventID, in, ""); err != nil {
		return nil, err
	}

	p := &prize.Prize{
		EventID:          eventID,
		RankFrom:         in.RankFrom,
		RankTo:           in.RankTo,
		PrizeName:        strings.TrimSpace(in.PrizeName),
		PrizeDescription: normalizedDescription(in.PrizeDescription),
	}
	if err := s.prizes.Create(ctx, p); err != nil {
		return nil, err
	}

	s.log.Info("prize created", "prize_id", p.ID, "event_id", eventID, "rank_from", p.RankFrom, "rank_to", p.RankTo)
	return p, nil
}

// Update replaces the mutable fields of an existing prize tier. Returns
// prize.ErrNotFound when the id does not exist. Admin-only at the
// transport layer.
func (s *Service) Update(ctx context.Context, eventID, id string, in Input) (*prize.Prize, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if err := s.checkNoOverlap(ctx, eventID, in, id); err != nil {
		return nil, err
	}

	p := &prize.Prize{
		ID:               id,
		EventID:          eventID,
		RankFrom:         in.RankFrom,
		RankTo:           in.RankTo,
		PrizeName:        strings.TrimSpace(in.PrizeName),
		PrizeDescription: normalizedDescription(in.PrizeDescription),
	}
	if err := s.prizes.Update(ctx, p); err != nil {
		return nil, err
	}

	s.log.Info("prize updated", "prize_id", p.ID, "event_id", eventID)
	return p, nil
}

// Delete removes a prize tier. Returns prize.ErrNotFound when the id does
// not exist. Admin-only at the transport layer.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.prizes.Delete(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	s.log.Info("prize deleted", "prize_id", id)
	return nil
}

// ListByEvent returns every prize tier for an event, ordered by
// rank_from. Backs both the admin and public listings.
func (s *Service) ListByEvent(ctx context.Context, eventID string) ([]*prize.Prize, error) {
	return s.prizes.ListByEvent(ctx, eventID)
}

// checkNoOverlap rejects a rank range that overlaps an existing prize
// tier for the same event, other than the tier being updated (excludeID
// is "" on create, when nothing is excluded).
func (s *Service) checkNoOverlap(ctx context.Context, eventID string, in Input, excludeID string) error {
	existing, err := s.prizes.ListByEvent(ctx, eventID)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.ID == excludeID {
			continue
		}
		if in.RankFrom <= p.RankTo && p.RankFrom <= in.RankTo {
			return fmt.Errorf("%w: rank range %d-%d overlaps an existing prize tier (%d-%d)", ErrValidation, in.RankFrom, in.RankTo, p.RankFrom, p.RankTo)
		}
	}
	return nil
}
