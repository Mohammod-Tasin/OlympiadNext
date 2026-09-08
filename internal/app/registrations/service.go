// Package registrations contains the application-layer orchestration for
// manual exam-registration payments: a student submits a bKash/Nagad
// transaction id, and an admin approves or rejects it. It depends only on
// domain interfaces, never on concrete infrastructure.
//
// Authorization (student vs. admin) is enforced at the HTTP middleware
// layer, not here.
package registrations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/domain/registration"
)

// ErrValidation is returned for a malformed submission. Callers map it to
// a 400. The other error conditions (duplicate TrxID, already registered,
// event not active) have their own sentinels so the handler can give each
// a specific status and message.
var (
	ErrValidation     = errors.New("registrations: invalid submission")
	ErrEventNotFound  = errors.New("registrations: event not found")
	ErrEventNotActive = errors.New("registrations: event is not open for registration")
)

// A bKash/Nagad transaction id is short and alphanumeric. bKash uses
// 10 uppercase alphanumerics; Nagad is similar. Accept a slightly wider
// band so a format tweak on their side does not lock students out — the
// admin still checks every id by hand against their statement.
var transactionIDPattern = regexp.MustCompile(`^[A-Z0-9]{6,20}$`)

// A Bangladeshi mobile number in local form: 01, then an operator digit,
// then 8 more digits. normalizeSenderNumber strips a +880/880 prefix
// first, so a number entered either way ends up stored the same.
var senderNumberPattern = regexp.MustCompile(`^01[3-9][0-9]{8}$`)

// RegisterInput is the student-supplied payload behind
// POST /api/user/registrations.
type RegisterInput struct {
	EventID       string
	PaymentMethod string
	SenderNumber  string
	TransactionID string
}

type Service struct {
	registrations registration.Repository
	events        event.Repository
	log           *slog.Logger
}

func NewService(registrations registration.Repository, events event.Repository, log *slog.Logger) *Service {
	return &Service{registrations: registrations, events: events, log: log}
}

// Register records a new pending registration for the authenticated
// student. It rejects a submission for an unknown or inactive event, a
// malformed payment method / number / transaction id, a transaction id
// that has already been used, and a second registration for an event the
// student is already registered for.
func (s *Service) Register(ctx context.Context, userID string, in RegisterInput) (*registration.Registration, error) {
	method := registration.PaymentMethod(strings.ToLower(strings.TrimSpace(in.PaymentMethod)))
	if !method.Valid() {
		return nil, fmt.Errorf("%w: payment_method must be 'bkash' or 'nagad'", ErrValidation)
	}

	senderNumber, ok := normalizeSenderNumber(in.SenderNumber)
	if !ok {
		return nil, fmt.Errorf("%w: sender_number must be a valid Bangladeshi mobile number", ErrValidation)
	}

	trxID := strings.ToUpper(strings.TrimSpace(in.TransactionID))
	if !transactionIDPattern.MatchString(trxID) {
		return nil, fmt.Errorf("%w: transaction_id is not a valid bKash/Nagad transaction id", ErrValidation)
	}

	ev, err := s.events.FindByID(ctx, strings.TrimSpace(in.EventID))
	if err != nil {
		if errors.Is(err, event.ErrNotFound) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("registrations: load event failed: %w", err)
	}
	if !ev.IsActive {
		return nil, ErrEventNotActive
	}

	reg := &registration.Registration{
		UserID:        userID,
		EventID:       ev.ID,
		PaymentMethod: method,
		SenderNumber:  senderNumber,
		TransactionID: trxID,
	}
	if err := s.registrations.Create(ctx, reg); err != nil {
		// Duplicate-TrxID and already-registered are expected user errors,
		// not failures — let them pass through unwrapped for the handler.
		if errors.Is(err, registration.ErrDuplicateTransactionID) ||
			errors.Is(err, registration.ErrAlreadyRegistered) {
			return nil, err
		}
		return nil, fmt.Errorf("registrations: create failed: %w", err)
	}

	s.log.Info("exam registration submitted", "registration_id", reg.ID, "user_id", userID, "event_id", ev.ID, "method", method)
	return reg, nil
}

// Get returns a single registration by id, or registration.ErrNotFound.
// The admit-card upload handler uses it to learn the owner and current
// status before storing a file.
func (s *Service) Get(ctx context.Context, id string) (*registration.Registration, error) {
	return s.registrations.FindByID(ctx, strings.TrimSpace(id))
}

// AttachAdmitCard records an admin-uploaded admit-card path against an
// approved registration and returns the event title (best effort, for the
// notification copy). The repository enforces the 'approved' precondition,
// returning registration.ErrNotApproved otherwise.
func (s *Service) AttachAdmitCard(ctx context.Context, id, url string) (eventTitle string, err error) {
	id = strings.TrimSpace(id)
	if err := s.registrations.SetAdmitCard(ctx, id, url); err != nil {
		return "", err
	}

	s.log.Info("admit card attached", "registration_id", id, "url", url)

	if reg, findErr := s.registrations.FindByID(ctx, id); findErr == nil {
		if ev, evErr := s.events.FindByID(ctx, reg.EventID); evErr == nil {
			eventTitle = ev.Title
		}
	}
	return eventTitle, nil
}

// ListForUser returns the caller's own registrations, newest first, each
// carrying its event title.
func (s *Service) ListForUser(ctx context.Context, userID string) ([]*registration.Detail, error) {
	return s.registrations.ListByUser(ctx, userID)
}

// ListForReview returns the admin review queue. Empty status and eventID
// values omit their corresponding filters; limit caps the result. A supplied
// eventID must refer to a real event so callers can distinguish an empty
// review queue from a mistyped event ID.
func (s *Service) ListForReview(ctx context.Context, status registration.Status, eventID string, limit int) ([]*registration.Detail, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID != "" {
		if _, err := s.events.FindByID(ctx, eventID); err != nil {
			if errors.Is(err, event.ErrNotFound) {
				return nil, ErrEventNotFound
			}
			return nil, fmt.Errorf("registrations: load event for review filter failed: %w", err)
		}
	}
	return s.registrations.ListByStatus(ctx, status, eventID, limit)
}

// Review records an admin's approve/reject decision. decision must be
// 'approved' or 'rejected'; 'pending' or anything else is a validation
// error. adminID is stored as reviewed_by.
func (s *Service) Review(ctx context.Context, id, adminID string, decision registration.Status) error {
	if decision != registration.StatusApproved && decision != registration.StatusRejected {
		return fmt.Errorf("%w: status must be 'approved' or 'rejected'", ErrValidation)
	}
	if err := s.registrations.Review(ctx, id, adminID, decision); err != nil {
		return err
	}
	s.log.Info("exam registration reviewed", "registration_id", id, "reviewed_by", adminID, "status", decision)
	return nil
}

// Unreject is the narrow admin correction path for a mistaken rejection.
// It permits only rejected -> pending and intentionally does not allow an
// approved or already-pending registration to be edited this way.
func (s *Service) Unreject(ctx context.Context, id, adminID string) error {
	reg, err := s.registrations.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !reg.Status.CanUnreject() {
		return registration.ErrInvalidUnrejectTransition
	}

	// The repository repeats the status predicate in SQL, so a concurrent
	// review cannot turn this read-then-write check into a broader transition.
	if err := s.registrations.Unreject(ctx, id); err != nil {
		return err
	}

	s.log.Info("exam registration unrejected", "admin_id", adminID, "registration_id", id, "from_status", registration.StatusRejected, "to_status", registration.StatusPending)
	return nil
}

// normalizeSenderNumber accepts a Bangladeshi mobile number written as
// 01XXXXXXXXX, +8801XXXXXXXXX or 8801XXXXXXXXX (with or without spaces and
// dashes) and returns it in the canonical 01XXXXXXXXX form. ok is false
// when the input is not a recognizable BD mobile number.
func normalizeSenderNumber(raw string) (string, bool) {
	n := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	n = strings.TrimPrefix(n, "+")
	if strings.HasPrefix(n, "880") {
		n = "0" + strings.TrimPrefix(n, "880")
	}
	if !senderNumberPattern.MatchString(n) {
		return "", false
	}
	return n, true
}
