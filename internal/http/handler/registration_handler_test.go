package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"olympiadnext/internal/app/registrations"
	"olympiadnext/internal/domain/event"
	"olympiadnext/internal/domain/registration"
	"olympiadnext/internal/http/dto"
)

type listReviewRegistrationRepo struct {
	registration.Repository
	listByStatus func(context.Context, registration.Status, string, int) ([]*registration.Detail, error)
}

func (f listReviewRegistrationRepo) ListByStatus(ctx context.Context, status registration.Status, eventID string, limit int) ([]*registration.Detail, error) {
	return f.listByStatus(ctx, status, eventID, limit)
}

type listReviewEventRepo struct {
	event.Repository
	findByID func(context.Context, string) (*event.Event, error)
}

func (f listReviewEventRepo) FindByID(ctx context.Context, id string) (*event.Event, error) {
	return f.findByID(ctx, id)
}

// admitCardRegistrationRepo is a registration.Repository whose only wired
// method is FindByID, for the admit-card precondition test.
type admitCardRegistrationRepo struct {
	registration.Repository
	findByID func(context.Context, string) (*registration.Registration, error)
}

func (f admitCardRegistrationRepo) FindByID(ctx context.Context, id string) (*registration.Registration, error) {
	return f.findByID(ctx, id)
}

// TestUploadAdmitCard_RejectsNonApprovedRegistration checks the 409 guard:
// an admit card can only be issued for an approved registration, and the
// handler must bail out before touching storage or the notifier (both nil
// here, so any use panics).
func TestUploadAdmitCard_RejectsNonApprovedRegistration(t *testing.T) {
	for _, status := range []registration.Status{
		registration.StatusPending, registration.StatusRejected,
	} {
		t.Run(string(status), func(t *testing.T) {
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			regRepo := admitCardRegistrationRepo{findByID: func(_ context.Context, _ string) (*registration.Registration, error) {
				return &registration.Registration{ID: "reg-1", UserID: "u1", Status: status}, nil
			}}
			h := NewRegistrationHandler(registrations.NewService(regRepo, nil, log), nil, nil, log)

			req := httptest.NewRequest(http.MethodPost, "/api/admin/registrations/reg-1/admit-card", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "reg-1")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			res := httptest.NewRecorder()

			h.UploadAdmitCard(res, req)

			if res.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d; body: %s", res.Code, http.StatusConflict, res.Body.String())
			}
		})
	}
}

func newListReviewHandler(regRepo registration.Repository, eventRepo event.Repository) *RegistrationHandler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// storage and notifier are unused on the list/review paths under test.
	return NewRegistrationHandler(registrations.NewService(regRepo, eventRepo, log), nil, nil, log)
}

func TestListForReview_AppliesStatusAndEventIDFilters(t *testing.T) {
	var gotStatus registration.Status
	var gotEventID string
	var gotLimit int
	studentName := "Student Name"
	h := newListReviewHandler(
		listReviewRegistrationRepo{listByStatus: func(_ context.Context, status registration.Status, eventID string, limit int) ([]*registration.Detail, error) {
			gotStatus, gotEventID, gotLimit = status, eventID, limit
			return []*registration.Detail{{
				Registration: registration.Registration{
					ID:            "reg-1",
					EventID:       "event-1",
					PaymentMethod: registration.MethodBkash,
					SenderNumber:  "01712345678",
					TransactionID: "ABC123",
					Status:        registration.StatusPending,
					CreatedAt:     time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				},
				StudentName:  &studentName,
				StudentEmail: "student@example.com",
				EventTitle:   "Math Olympiad",
			}}, nil
		}},
		listReviewEventRepo{findByID: func(_ context.Context, id string) (*event.Event, error) {
			if id != "event-1" {
				t.Fatalf("event lookup ID = %q, want event-1", id)
			}
			return &event.Event{ID: id}, nil
		}},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/registrations?status=pending&event_id=event-1", nil)
	res := httptest.NewRecorder()
	h.ListForReview(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", res.Code, http.StatusOK, res.Body.String())
	}
	if gotStatus != registration.StatusPending || gotEventID != "event-1" || gotLimit != maxRegistrationListLimit {
		t.Errorf("repository filters = (%q, %q, %d), want (pending, event-1, %d)", gotStatus, gotEventID, gotLimit, maxRegistrationListLimit)
	}

	var body dto.AdminRegistrationListResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Count != 1 || len(body.Registrations) != 1 {
		t.Fatalf("response count = %d, registrations = %d; want one", body.Count, len(body.Registrations))
	}
	got := body.Registrations[0]
	if got.StudentName == nil || *got.StudentName != studentName || got.StudentEmail != "student@example.com" || got.EventTitle != "Math Olympiad" {
		t.Errorf("joined response fields not preserved: %+v", got)
	}
}

func TestListForReview_UnknownEventReturnsNotFound(t *testing.T) {
	calledList := false
	h := newListReviewHandler(
		listReviewRegistrationRepo{listByStatus: func(_ context.Context, _ registration.Status, _ string, _ int) ([]*registration.Detail, error) {
			calledList = true
			return nil, nil
		}},
		listReviewEventRepo{findByID: func(_ context.Context, _ string) (*event.Event, error) {
			return nil, event.ErrNotFound
		}},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/registrations?event_id=missing-event", nil)
	res := httptest.NewRecorder()
	h.ListForReview(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", res.Code, http.StatusNotFound, res.Body.String())
	}
	if calledList {
		t.Error("registration repository must not be queried for an unknown event")
	}
}
