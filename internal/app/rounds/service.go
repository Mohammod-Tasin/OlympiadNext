// Package rounds contains the application-layer orchestration for an
// event's sequential rounds: admin CRUD and status transitions, the
// student-facing entry gate, and the admin candidate/decision workflow.
// It depends only on domain interfaces, never on concrete infrastructure.
//
// Authorization (admin vs. client) is enforced at the HTTP middleware
// layer, not here: the service assumes its caller has already been
// vetted and focuses solely on validation, eligibility, and persistence.
package rounds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/domain/round"
)

// ErrValidation is returned when caller-supplied round or participant
// data is incomplete or malformed. Callers should map it to a 400
// response.
var ErrValidation = errors.New("rounds: invalid round data")

// Input is the mutable state of a round, supplied by an admin on both
// create and update. Status is deliberately absent: it only moves via the
// explicit start/end transitions.
type Input struct {
	RoundOrder      int
	RoundName       string
	StartAt         time.Time
	DurationMinutes int
}

func (in Input) validate() error {
	if strings.TrimSpace(in.RoundName) == "" {
		return fmt.Errorf("%w: round_name is required", ErrValidation)
	}
	if in.RoundOrder <= 0 {
		return fmt.Errorf("%w: round_order must be positive", ErrValidation)
	}
	if in.StartAt.IsZero() {
		return fmt.Errorf("%w: start_at is required", ErrValidation)
	}
	if in.DurationMinutes <= 0 {
		return fmt.Errorf("%w: duration_minutes must be greater than zero", ErrValidation)
	}
	return nil
}

// ParticipantDecision is one admin decision submitted to SetParticipants.
type ParticipantDecision struct {
	UserID string
	Status round.ParticipantStatus
}

// Candidate is one student eligible for a round's next decision, with
// their existing decision in that round (if any) for a pre-ticking admin
// UI.
type Candidate struct {
	UserID         string
	FullName       *string
	Email          string
	ExistingStatus *string
}

type Service struct {
	rounds        round.Repository
	registrations registration.Repository
	log           *slog.Logger
}

func NewService(rounds round.Repository, registrations registration.Repository, log *slog.Logger) *Service {
	return &Service{rounds: rounds, registrations: registrations, log: log}
}

// CreateRound persists a new round for an event, always starting
// 'upcoming'. Admin-only at the transport layer.
func (s *Service) CreateRound(ctx context.Context, eventID string, in Input) (*round.Round, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	rnd := &round.Round{
		EventID:         eventID,
		RoundOrder:      in.RoundOrder,
		RoundName:       strings.TrimSpace(in.RoundName),
		StartAt:         in.StartAt,
		DurationMinutes: in.DurationMinutes,
	}
	if err := s.rounds.Create(ctx, rnd); err != nil {
		return nil, err
	}

	s.log.Info("round created", "round_id", rnd.ID, "event_id", eventID, "round_order", rnd.RoundOrder)
	return rnd, nil
}

// UpdateRound replaces the order/name/start/duration of an existing
// round. Returns round.ErrNotFound when the id does not exist. Admin-only
// at the transport layer.
func (s *Service) UpdateRound(ctx context.Context, id string, in Input) (*round.Round, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	existing, err := s.rounds.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	existing.RoundOrder = in.RoundOrder
	existing.RoundName = strings.TrimSpace(in.RoundName)
	existing.StartAt = in.StartAt
	existing.DurationMinutes = in.DurationMinutes

	if err := s.rounds.Update(ctx, existing); err != nil {
		return nil, err
	}

	s.log.Info("round updated", "round_id", existing.ID)
	return existing, nil
}

// DeleteRound removes a round. Admin-only at the transport layer.
func (s *Service) DeleteRound(ctx context.Context, id string) error {
	if err := s.rounds.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("round deleted", "round_id", id)
	return nil
}

// ListRoundsByEvent returns every round for an event, ordered by
// round_order. Public.
func (s *Service) ListRoundsByEvent(ctx context.Context, eventID string) ([]*round.Round, error) {
	return s.rounds.ListByEvent(ctx, eventID)
}

// StartRound moves a round from 'upcoming' to 'ongoing'. Returns
// round.ErrInvalidTransition if it is not currently upcoming. Admin-only
// at the transport layer.
func (s *Service) StartRound(ctx context.Context, id string) (*round.Round, error) {
	if err := s.rounds.SetStatus(ctx, id, round.StatusUpcoming, round.StatusOngoing); err != nil {
		return nil, err
	}
	s.log.Info("round started", "round_id", id)
	return s.rounds.FindByID(ctx, id)
}

// EndRound moves a round from 'ongoing' to 'ended'. Returns
// round.ErrInvalidTransition if it is not currently ongoing. Admin-only
// at the transport layer.
func (s *Service) EndRound(ctx context.Context, id string) (*round.Round, error) {
	if err := s.rounds.SetStatus(ctx, id, round.StatusOngoing, round.StatusEnded); err != nil {
		return nil, err
	}
	s.log.Info("round ended", "round_id", id)
	return s.rounds.FindByID(ctx, id)
}

// isEligibleForRound reports whether userID may enter rnd, independent of
// rnd's own status/timing: round 1 requires an approved event
// registration; round N>1 requires a 'qualified' decision in round N-1.
// A missing round N-1, or a missing/non-qualifying decision in it, both
// mean "not eligible" rather than an error.
func (s *Service) isEligibleForRound(ctx context.Context, userID string, rnd *round.Round) (bool, error) {
	if rnd.RoundOrder == 1 {
		return s.registrations.ExistsApprovedForUserEvent(ctx, userID, rnd.EventID)
	}

	prev, err := s.rounds.FindByEventAndOrder(ctx, rnd.EventID, rnd.RoundOrder-1)
	if err != nil {
		if errors.Is(err, round.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	p, err := s.rounds.GetParticipant(ctx, prev.ID, userID)
	if err != nil {
		if errors.Is(err, round.ErrParticipantNotFound) {
			return false, nil
		}
		return false, err
	}
	return p.Status == round.ParticipantQualified, nil
}

// EnterRound is the real security gate behind POST /rounds/{id}/enter:
// never trust a client-side countdown. It returns (false, reason, nil)
// for every ineligible case and only a non-nil err for an unknown round
// or an infrastructure failure.
func (s *Service) EnterRound(ctx context.Context, roundID, userID string) (bool, string, error) {
	rnd, err := s.rounds.FindByID(ctx, roundID)
	if err != nil {
		return false, "", err
	}
	if rnd.Status != round.StatusOngoing {
		return false, "round is not ongoing", nil
	}

	eligible, err := s.isEligibleForRound(ctx, userID, rnd)
	if err != nil {
		return false, "", err
	}
	if !eligible {
		if rnd.RoundOrder == 1 {
			return false, "no active registration for this event", nil
		}
		return false, "not qualified from the previous round", nil
	}
	return true, "", nil
}

// YourStatus backs the public round listing's per-caller your_status:
// an already-recorded decision is reported verbatim; otherwise it is one
// of "not_eligible" (round 1, no approved registration), "waiting"
// (round N>1, not yet qualified from the previous round), "ready"
// (eligible, round ongoing, start_at passed) or "locked" (eligible, but
// not yet time or the round isn't ongoing).
func (s *Service) YourStatus(ctx context.Context, userID string, rnd *round.Round) (string, error) {
	p, err := s.rounds.GetParticipant(ctx, rnd.ID, userID)
	if err == nil {
		return string(p.Status), nil
	}
	if !errors.Is(err, round.ErrParticipantNotFound) {
		return "", err
	}

	eligible, err := s.isEligibleForRound(ctx, userID, rnd)
	if err != nil {
		return "", err
	}
	if !eligible {
		if rnd.RoundOrder == 1 {
			return "not_eligible", nil
		}
		return "waiting", nil
	}

	if rnd.Status == round.StatusOngoing && !time.Now().UTC().Before(rnd.StartAt) {
		return "ready", nil
	}
	return "locked", nil
}

// maxCandidateList caps the round-1 candidate source query
// (registrations.ListByStatus), which otherwise has no limit parameter of
// its own on the caller's side.
const maxCandidateList = 5000

// GetCandidates returns the students eligible for the next decision in
// rnd, each with their existing decision in this round (if any) so an
// admin UI can pre-tick it. Only callable once the round has ended.
func (s *Service) GetCandidates(ctx context.Context, roundID string) ([]Candidate, error) {
	rnd, err := s.rounds.FindByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if rnd.Status != round.StatusEnded {
		return nil, round.ErrNotEnded
	}

	var candidates []Candidate
	if rnd.RoundOrder == 1 {
		regs, err := s.registrations.ListByStatus(ctx, registration.StatusApproved, rnd.EventID, maxCandidateList)
		if err != nil {
			return nil, err
		}
		for _, d := range regs {
			candidates = append(candidates, Candidate{UserID: d.UserID, FullName: d.StudentName, Email: d.StudentEmail})
		}
	} else {
		prev, err := s.rounds.FindByEventAndOrder(ctx, rnd.EventID, rnd.RoundOrder-1)
		if err != nil && !errors.Is(err, round.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			details, err := s.rounds.ListParticipantsWithUser(ctx, prev.ID, round.ParticipantQualified)
			if err != nil {
				return nil, err
			}
			for _, d := range details {
				candidates = append(candidates, Candidate{UserID: d.UserID, FullName: d.StudentName, Email: d.StudentEmail})
			}
		}
	}

	existing, err := s.rounds.ListParticipantsByRound(ctx, roundID)
	if err != nil {
		return nil, err
	}
	existingByUser := make(map[string]round.ParticipantStatus, len(existing))
	for _, p := range existing {
		existingByUser[p.UserID] = p.Status
	}
	for i := range candidates {
		if st, ok := existingByUser[candidates[i].UserID]; ok {
			label := string(st)
			candidates[i].ExistingStatus = &label
		}
	}
	return candidates, nil
}

// SetParticipants bulk-records this round's decisions. Only callable once
// the round has ended; 'winner' is only accepted on an event's
// highest-numbered round.
func (s *Service) SetParticipants(ctx context.Context, roundID string, decisions []ParticipantDecision) (int, error) {
	rnd, err := s.rounds.FindByID(ctx, roundID)
	if err != nil {
		return 0, err
	}
	if rnd.Status != round.StatusEnded {
		return 0, round.ErrNotEnded
	}
	if len(decisions) == 0 {
		return 0, fmt.Errorf("%w: at least one participant decision is required", ErrValidation)
	}

	maxOrder, err := s.rounds.MaxRoundOrder(ctx, rnd.EventID)
	if err != nil {
		return 0, err
	}
	isFinalRound := rnd.RoundOrder == maxOrder

	participants := make([]round.Participant, 0, len(decisions))
	for _, d := range decisions {
		userID := strings.TrimSpace(d.UserID)
		if userID == "" {
			return 0, fmt.Errorf("%w: user_id is required", ErrValidation)
		}
		if !d.Status.Valid() {
			return 0, fmt.Errorf("%w: status must be one of qualified, eliminated, winner", ErrValidation)
		}
		if d.Status == round.ParticipantWinner && !isFinalRound {
			return 0, fmt.Errorf("%w: winner status is only allowed on the final round", ErrValidation)
		}
		participants = append(participants, round.Participant{RoundID: roundID, UserID: userID, Status: d.Status})
	}

	if err := s.rounds.UpsertParticipants(ctx, roundID, participants); err != nil {
		return 0, err
	}

	s.log.Info("round participants decided", "round_id", roundID, "count", len(participants))
	return len(participants), nil
}
