package registrations

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/domain/registration"
)

// The fakes embed the domain interface, so any method a test path does not
// use stays nil and panics loudly if it is ever called.

type fakeRegistrationRepo struct {
	registration.Repository
	create func(ctx context.Context, r *registration.Registration) error
}

func (f fakeRegistrationRepo) Create(ctx context.Context, r *registration.Registration) error {
	return f.create(ctx, r)
}

type fakeEventRepo struct {
	event.Repository
	findByID func(ctx context.Context, id string) (*event.Event, error)
}

func (f fakeEventRepo) FindByID(ctx context.Context, id string) (*event.Event, error) {
	return f.findByID(ctx, id)
}

func newService(regRepo registration.Repository, evRepo event.Repository) *Service {
	return NewService(regRepo, evRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func activeEvent() *event.Event {
	return &event.Event{ID: "event-1", Title: "Math Olympiad", IsActive: true}
}

func validInput() RegisterInput {
	return RegisterInput{
		EventID:       "event-1",
		PaymentMethod: "bkash",
		SenderNumber:  "01712345678",
		TransactionID: "abc123xyz",
	}
}

func TestRegister_HappyPath_NormalizesAndPersists(t *testing.T) {
	var saved *registration.Registration
	svc := newService(
		fakeRegistrationRepo{create: func(_ context.Context, r *registration.Registration) error {
			r.ID = "reg-1"
			r.Status = registration.StatusPending
			saved = r
			return nil
		}},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) { return activeEvent(), nil }},
	)

	in := validInput()
	in.TransactionID = "  abc123xyz  "
	in.SenderNumber = "+8801712345678"

	reg, err := svc.Register(context.Background(), "user-1", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.TransactionID != "ABC123XYZ" {
		t.Errorf("transaction id not upper-cased/trimmed: %q", reg.TransactionID)
	}
	if saved.SenderNumber != "01712345678" {
		t.Errorf("sender number not normalized: %q", saved.SenderNumber)
	}
	if saved.UserID != "user-1" || saved.EventID != "event-1" {
		t.Errorf("wrong owner/event: %+v", saved)
	}
}

func TestRegister_RejectsInactiveEvent(t *testing.T) {
	svc := newService(
		fakeRegistrationRepo{create: func(_ context.Context, _ *registration.Registration) error {
			t.Fatal("Create must not be called for an inactive event")
			return nil
		}},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) {
			e := activeEvent()
			e.IsActive = false
			return e, nil
		}},
	)

	if _, err := svc.Register(context.Background(), "user-1", validInput()); !errors.Is(err, ErrEventNotActive) {
		t.Fatalf("want ErrEventNotActive, got %v", err)
	}
}

func TestRegister_RejectsUnknownEvent(t *testing.T) {
	svc := newService(
		fakeRegistrationRepo{},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) {
			return nil, event.ErrNotFound
		}},
	)

	if _, err := svc.Register(context.Background(), "user-1", validInput()); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("want ErrEventNotFound, got %v", err)
	}
}

func TestRegister_PropagatesDuplicateTransactionID(t *testing.T) {
	svc := newService(
		fakeRegistrationRepo{create: func(_ context.Context, _ *registration.Registration) error {
			return registration.ErrDuplicateTransactionID
		}},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) { return activeEvent(), nil }},
	)

	if _, err := svc.Register(context.Background(), "user-1", validInput()); !errors.Is(err, registration.ErrDuplicateTransactionID) {
		t.Fatalf("want ErrDuplicateTransactionID unwrapped, got %v", err)
	}
}

func TestRegister_PropagatesAlreadyRegistered(t *testing.T) {
	svc := newService(
		fakeRegistrationRepo{create: func(_ context.Context, _ *registration.Registration) error {
			return registration.ErrAlreadyRegistered
		}},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) { return activeEvent(), nil }},
	)

	if _, err := svc.Register(context.Background(), "user-1", validInput()); !errors.Is(err, registration.ErrAlreadyRegistered) {
		t.Fatalf("want ErrAlreadyRegistered unwrapped, got %v", err)
	}
}

func TestRegister_RejectsBadInput(t *testing.T) {
	svc := newService(
		fakeRegistrationRepo{create: func(_ context.Context, _ *registration.Registration) error {
			t.Fatal("Create must not be called when input validation fails")
			return nil
		}},
		fakeEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) {
			t.Fatal("event lookup must not happen before input is validated")
			return nil, nil
		}},
	)

	cases := map[string]func(*RegisterInput){
		"bad method":      func(in *RegisterInput) { in.PaymentMethod = "rocket" },
		"bad sender":      func(in *RegisterInput) { in.SenderNumber = "12345" },
		"empty trx":       func(in *RegisterInput) { in.TransactionID = "" },
		"trx with symbol": func(in *RegisterInput) { in.TransactionID = "abc-123" },
	}
	for name, mangle := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mangle(&in)
			if _, err := svc.Register(context.Background(), "user-1", in); !errors.Is(err, ErrValidation) {
				t.Fatalf("want ErrValidation, got %v", err)
			}
		})
	}
}

func TestReview_RejectsNonDecisionStatus(t *testing.T) {
	svc := newService(fakeRegistrationRepo{}, fakeEventRepo{})
	if err := svc.Review(context.Background(), "reg-1", "admin-1", registration.StatusPending); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation for a non-decision status, got %v", err)
	}
}
