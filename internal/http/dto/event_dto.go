package dto

import "time"

// EventRequest is the admin-supplied payload for both create and update.
type EventRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ImageURL    string    `json:"image_url"`
	EventDate   time.Time `json:"event_date"`
	IsActive    bool      `json:"is_active"`
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
