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
}

type EventResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ImageURL    string    `json:"image_url"`
	EventDate   time.Time `json:"event_date"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UploadResponse is returned by POST /api/admin/events/upload; the value
// is what the admin then submits as EventRequest.ImageURL.
type UploadResponse struct {
	ImageURL string `json:"image_url"`
}
