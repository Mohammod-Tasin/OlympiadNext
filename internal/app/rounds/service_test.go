package rounds

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/domain/round"
)

// The fakes embed the domain interface, so any method a test path does not
// use stays nil and panics loudly if it is ever called.

type fakeRoundRepo struct {
	round.Repository
	findByID            func(ctx context.Context, id string) (*round.Round, error)
	findByEventAndOrder func(ctx context.Context, eventID string, order int) (*round.Round, error)
	getParticipant      func(ctx context.Context, roundID, userID string) (*round.Participant, error)
}

func (f fakeRoundRepo) FindByID(ctx context.Context, id string) (*round.Round, error) {
	return f.findByID(ctx, id)
}

func (f fakeRoundRepo) FindByEventAndOrder(ctx context.Context, eventID string, order int) (*round.Round, error) {
	return f.findByEventAndOrder(ctx, eventID, order)
}

func (f fakeRoundRepo) GetParticipant(ctx context.Context, roundID, userID string) (*round.Participant, error) {
	return f.getParticipant(ctx, roundID, userID)
}

type fakeRegRepo struct {
	registration.Repository
	existsApproved func(ctx context.Context, userID, eventID string) (bool, error)
}

func (f fakeRegRepo) ExistsApprovedForUserEvent(ctx context.Context, userID, eventID string) (bool, error) {
	return f.existsApproved(ctx, userID, eventID)
}

func newRoundsService(rr round.Repository, gr registration.Repository) *Service {
	return NewService(rr, gr, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestEnterRound_DeniesOutsideDurationWindow(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name       string
		startAt    time.Time
		wantReason string
	}{
		{"before start", now.Add(10 * time.Minute), "round has not started yet"},
		{"after duration elapsed", now.Add(-90 * time.Minute), "round has ended"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rnd := &round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusOngoing, StartAt: tc.startAt, DurationMinutes: 60}
			svc := newRoundsService(
				fakeRoundRepo{findByID: func(_ context.Context, _ string) (*round.Round, error) { return rnd, nil }},
				fakeRegRepo{},
			)

			allowed, reason, err := svc.EnterRound(context.Background(), "r1", "u1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed {
				t.Fatalf("expected entry to be denied")
			}
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

func TestEnterRound_AllowsWithinDurationWindow(t *testing.T) {
	now := time.Now().UTC()
	rnd := &round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusOngoing, StartAt: now.Add(-10 * time.Minute), DurationMinutes: 60}
	svc := newRoundsService(
		fakeRoundRepo{findByID: func(_ context.Context, _ string) (*round.Round, error) { return rnd, nil }},
		fakeRegRepo{existsApproved: func(_ context.Context, _, _ string) (bool, error) { return true, nil }},
	)

	allowed, reason, err := svc.EnterRound(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected entry to be allowed, got reason %q", reason)
	}
}

func TestYourStatus_EliminatedInPriorRound_IsNotWaiting(t *testing.T) {
	// Round 2, no participant row yet in round 2, but round 1 recorded
	// this student as eliminated: your_status for round 2 must say so,
	// not "waiting" (which implies the decision is still pending).
	round2 := &round.Round{ID: "r2", EventID: "e1", RoundOrder: 2, Status: round.StatusUpcoming}
	round1 := &round.Round{ID: "r1", EventID: "e1", RoundOrder: 1}
	svc := newRoundsService(
		fakeRoundRepo{
			findByEventAndOrder: func(_ context.Context, _ string, order int) (*round.Round, error) { return round1, nil },
			getParticipant: func(_ context.Context, roundID, _ string) (*round.Participant, error) {
				return &round.Participant{RoundID: roundID, Status: round.ParticipantEliminated}, nil
			},
		},
		fakeRegRepo{},
	)

	got, err := svc.YourStatus(context.Background(), "u1", round2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "eliminated" {
		t.Fatalf("your_status = %q, want %q", got, "eliminated")
	}
}

func TestYourStatus_PriorRoundNotYetDecided_IsWaiting(t *testing.T) {
	round2 := &round.Round{ID: "r2", EventID: "e1", RoundOrder: 2, Status: round.StatusUpcoming}
	round1 := &round.Round{ID: "r1", EventID: "e1", RoundOrder: 1}
	svc := newRoundsService(
		fakeRoundRepo{
			findByEventAndOrder: func(_ context.Context, _ string, order int) (*round.Round, error) { return round1, nil },
			getParticipant: func(_ context.Context, _, _ string) (*round.Participant, error) {
				return nil, round.ErrParticipantNotFound
			},
		},
		fakeRegRepo{},
	)

	got, err := svc.YourStatus(context.Background(), "u1", round2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "waiting" {
		t.Fatalf("your_status = %q, want %q", got, "waiting")
	}
}

func TestYourStatus_EndedRoundWithNoDecision_IsNotReadyOrLocked(t *testing.T) {
	// Eligible for round 1 (approved registration), round 1 has ended, but
	// no round_participants row was recorded for this student.
	round1 := &round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusEnded}
	svc := newRoundsService(
		fakeRoundRepo{
			getParticipant: func(_ context.Context, _, _ string) (*round.Participant, error) {
				return nil, round.ErrParticipantNotFound
			},
		},
		fakeRegRepo{existsApproved: func(_ context.Context, _, _ string) (bool, error) { return true, nil }},
	)

	got, err := svc.YourStatus(context.Background(), "u1", round1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "ready" || got == "locked" || got == "waiting" {
		t.Fatalf("your_status = %q, want a status distinguishing an undecided-but-ended round", got)
	}
	if got != "eliminated" {
		t.Fatalf("your_status = %q, want %q", got, "eliminated")
	}
}
