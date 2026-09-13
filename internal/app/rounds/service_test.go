package rounds

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/domain/round"
	"olympiadnext/internal/domain/user"
)

// The fakes embed the domain interface, so any method a test path does not
// use stays nil and panics loudly if it is ever called.

// testLevel is the level every fake user and test round shares by default,
// so existing eligibility tests (which predate level-scoping) keep passing
// without every one of them having to thread a matching level through.
const testLevel = "Junior"

type fakeRoundRepo struct {
	round.Repository
	findByID            func(ctx context.Context, id string) (*round.Round, error)
	findByEventAndOrder func(ctx context.Context, eventID, level string, order int) (*round.Round, error)
	getParticipant      func(ctx context.Context, roundID, userID string) (*round.Participant, error)
}

func (f fakeRoundRepo) FindByID(ctx context.Context, id string) (*round.Round, error) {
	return f.findByID(ctx, id)
}

func (f fakeRoundRepo) FindByEventAndOrder(ctx context.Context, eventID, level string, order int) (*round.Round, error) {
	return f.findByEventAndOrder(ctx, eventID, level, order)
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

// fakeUserRepo always returns a user at testLevel with a 'verified'
// status, so isEligibleForRound's level and verification checks pass for
// every test round below (which also default to testLevel via testRound)
// unless a test overrides verificationStatus to exercise the KYC gate.
type fakeUserRepo struct {
	user.Repository
	verificationStatus user.VerificationStatus
}

func (f fakeUserRepo) FindByID(ctx context.Context, id string) (*user.User, error) {
	level := testLevel
	status := f.verificationStatus
	if status == "" {
		status = user.VerificationVerified
	}
	return &user.User{ID: id, Level: &level, VerificationStatus: status}, nil
}

// testRound fills in Level: testLevel alongside the given fields, so every
// existing test round automatically matches fakeUserRepo's level.
func testRound(r round.Round) *round.Round {
	r.Level = testLevel
	return &r
}

func newRoundsService(rr round.Repository, gr registration.Repository) *Service {
	return NewService(rr, gr, fakeUserRepo{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
			rnd := testRound(round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusOngoing, StartAt: tc.startAt, DurationMinutes: 60})
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
	rnd := testRound(round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusOngoing, StartAt: now.Add(-10 * time.Minute), DurationMinutes: 60})
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

func TestEnterRound_DeniesLevelMismatch(t *testing.T) {
	// The round is "Secondary"; fakeUserRepo always returns testLevel
	// ("Junior"), so this must be denied with a level-specific reason even
	// though the round is otherwise ongoing and the student has an
	// approved registration.
	now := time.Now().UTC()
	rnd := &round.Round{
		ID: "r1", EventID: "e1", RoundOrder: 1, Level: "Secondary",
		Status: round.StatusOngoing, StartAt: now.Add(-10 * time.Minute), DurationMinutes: 60,
	}
	svc := newRoundsService(
		fakeRoundRepo{findByID: func(_ context.Context, _ string) (*round.Round, error) { return rnd, nil }},
		fakeRegRepo{existsApproved: func(_ context.Context, _, _ string) (bool, error) { return true, nil }},
	)

	allowed, reason, err := svc.EnterRound(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatalf("expected entry to be denied for a level mismatch")
	}
	if want := "this round is only open to Secondary students"; reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestEnterRound_DeniesUnverifiedIdentity(t *testing.T) {
	// Level matches and the registration is approved, but the student's
	// KYC review is still 'pending' (e.g. reset by a later level/
	// institution change) — must be denied with a verification-specific
	// reason, not the generic "no active registration" message.
	now := time.Now().UTC()
	rnd := testRound(round.Round{
		ID: "r1", EventID: "e1", RoundOrder: 1,
		Status: round.StatusOngoing, StartAt: now.Add(-10 * time.Minute), DurationMinutes: 60,
	})
	svc := NewService(
		fakeRoundRepo{findByID: func(_ context.Context, _ string) (*round.Round, error) { return rnd, nil }},
		fakeRegRepo{existsApproved: func(_ context.Context, _, _ string) (bool, error) { return true, nil }},
		fakeUserRepo{verificationStatus: user.VerificationPending},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	allowed, reason, err := svc.EnterRound(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatalf("expected entry to be denied for an unverified identity")
	}
	if want := "your identity verification is still under review"; reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestYourStatus_EliminatedInPriorRound_IsNotWaiting(t *testing.T) {
	// Round 2, no participant row yet in round 2, but round 1 recorded
	// this student as eliminated: your_status for round 2 must say so,
	// not "waiting" (which implies the decision is still pending).
	round2 := testRound(round.Round{ID: "r2", EventID: "e1", RoundOrder: 2, Status: round.StatusUpcoming})
	round1 := testRound(round.Round{ID: "r1", EventID: "e1", RoundOrder: 1})
	svc := newRoundsService(
		fakeRoundRepo{
			findByEventAndOrder: func(_ context.Context, _, _ string, order int) (*round.Round, error) { return round1, nil },
			getParticipant: func(_ context.Context, roundID, _ string) (*round.Participant, error) {
				return &round.Participant{RoundID: roundID, Status: round.ParticipantEliminated}, nil
			},
		},
		fakeRegRepo{},
	)

	got, _, err := svc.YourStatus(context.Background(), "u1", round2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "eliminated" {
		t.Fatalf("your_status = %q, want %q", got, "eliminated")
	}
}

func TestYourStatus_PriorRoundNotYetDecided_IsWaiting(t *testing.T) {
	round2 := testRound(round.Round{ID: "r2", EventID: "e1", RoundOrder: 2, Status: round.StatusUpcoming})
	round1 := testRound(round.Round{ID: "r1", EventID: "e1", RoundOrder: 1})
	svc := newRoundsService(
		fakeRoundRepo{
			findByEventAndOrder: func(_ context.Context, _, _ string, order int) (*round.Round, error) { return round1, nil },
			getParticipant: func(_ context.Context, _, _ string) (*round.Participant, error) {
				return nil, round.ErrParticipantNotFound
			},
		},
		fakeRegRepo{},
	)

	got, _, err := svc.YourStatus(context.Background(), "u1", round2)
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
	round1 := testRound(round.Round{ID: "r1", EventID: "e1", RoundOrder: 1, Status: round.StatusEnded})
	svc := newRoundsService(
		fakeRoundRepo{
			getParticipant: func(_ context.Context, _, _ string) (*round.Participant, error) {
				return nil, round.ErrParticipantNotFound
			},
		},
		fakeRegRepo{existsApproved: func(_ context.Context, _, _ string) (bool, error) { return true, nil }},
	)

	got, _, err := svc.YourStatus(context.Background(), "u1", round1)
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
