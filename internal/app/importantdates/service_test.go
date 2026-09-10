package importantdates

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"olympiadnext/internal/domain/importantdate"
)

// The fake embeds the domain interface, so any method a test path does not
// use stays nil and panics loudly if it is ever called.

type fakeImportantDateRepo struct {
	importantdate.Repository
	create func(ctx context.Context, d *importantdate.ImportantDate) error
}

func (f fakeImportantDateRepo) Create(ctx context.Context, d *importantdate.ImportantDate) error {
	return f.create(ctx, d)
}

func newService(repo importantdate.Repository) *Service {
	return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func validInput() Input {
	return Input{
		EventDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Title:     "Registration opens",
		DetailsEN: "Online registration opens",
		DetailsBN: "অনলাইন নিবন্ধন শুরু",
	}
}

func TestCreate_HappyPath_TrimsAndPersists(t *testing.T) {
	var saved *importantdate.ImportantDate
	svc := newService(fakeImportantDateRepo{create: func(_ context.Context, d *importantdate.ImportantDate) error {
		d.ID = "id-1"
		saved = d
		return nil
	}})

	in := validInput()
	in.Title = "  Registration opens  "

	d, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Title != "Registration opens" {
		t.Errorf("title not trimmed: %q", d.Title)
	}
	if saved.EventDate != in.EventDate {
		t.Errorf("event date not persisted: %v", saved.EventDate)
	}
}

func TestCreate_RejectsBadInput(t *testing.T) {
	svc := newService(fakeImportantDateRepo{create: func(_ context.Context, _ *importantdate.ImportantDate) error {
		t.Fatal("Create must not be called when input validation fails")
		return nil
	}})

	cases := map[string]func(*Input){
		"blank title":      func(in *Input) { in.Title = "   " },
		"blank details_en": func(in *Input) { in.DetailsEN = "" },
		"blank details_bn": func(in *Input) { in.DetailsBN = "" },
		"negative order":   func(in *Input) { in.DisplayOrder = -1 },
		"zero event_date":  func(in *Input) { in.EventDate = time.Time{} },
	}
	for name, mangle := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mangle(&in)
			if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrValidation) {
				t.Fatalf("want ErrValidation, got %v", err)
			}
		})
	}
}
