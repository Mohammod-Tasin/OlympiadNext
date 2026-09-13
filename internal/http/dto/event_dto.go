package dto

import "time"

// EventRequest is the admin-supplied payload for both create and update.
// EventDate is received as a string and parsed strictly as RFC3339 by the
// handler, so a malformed timestamp yields a clear 400 rather than a
// generic "invalid request body".
type EventRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	EventDate   string `json:"event_date"`
	IsActive    bool   `json:"is_active"`
	// Manual bKash/Nagad payment details for exam registration. Optional.
	BkashNumber     string `json:"bkash_number"`
	NagadNumber     string `json:"nagad_number"`
	RegistrationFee int    `json:"registration_fee"`
}

type EventResponse struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	ImageURL        string    `json:"image_url"`
	EventDate       time.Time `json:"event_date"`
	IsActive        bool      `json:"is_active"`
	BkashNumber     string    `json:"bkash_number"`
	NagadNumber     string    `json:"nagad_number"`
	RegistrationFee int       `json:"registration_fee"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// IsRegistered is true when the authenticated caller already has an
	// exam registration (any status) for this event. It is always false on
	// the admin create/update responses and on an unauthenticated client
	// request, so the frontend can rely on it being present.
	IsRegistered bool `json:"is_registered"`
}

// UploadResponse is returned by POST /api/admin/events/upload; the value
// is what the admin then submits as EventRequest.ImageURL.
type UploadResponse struct {
	ImageURL string `json:"image_url"`
}

// EventListResponse is the body of GET /api/client/events/all. IsRegistered
// is always false on every row — the route is unauthenticated, unlike the
// single-event GetActiveEvent/GetByID routes.
type EventListResponse struct {
	Events []EventResponse `json:"events"`
	Count  int             `json:"count"`
}
