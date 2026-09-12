package dto

import "time"

// RoundRequest is the admin-supplied payload for both create and update.
// StartAt is an RFC3339 string, parsed strictly by the handler like
// EventRequest.EventDate.
type RoundRequest struct {
	RoundOrder      int    `json:"round_order"`
	RoundName       string `json:"round_name"`
	StartAt         string `json:"start_at"`
	DurationMinutes int    `json:"duration_minutes"`
}

// RoundResponse is a round as seen by both the admin console and the
// client frontend. YourStatus is only populated on the public listing,
// and only when the request carried a valid access token.
type RoundResponse struct {
	ID              string    `json:"id"`
	EventID         string    `json:"event_id"`
	RoundOrder      int       `json:"round_order"`
	RoundName       string    `json:"round_name"`
	StartAt         time.Time `json:"start_at"`
	DurationMinutes int       `json:"duration_minutes"`
	Status          string    `json:"status"`
	YourStatus      *string   `json:"your_status,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RoundListResponse struct {
	Rounds []RoundResponse `json:"rounds"`
	Count  int             `json:"count"`
}

// CandidateResponse is one student eligible for a round's next decision.
// ExistingStatus is set when the admin has already decided this student
// in this round, so a UI can pre-tick it.
type CandidateResponse struct {
	UserID         string  `json:"user_id"`
	FullName       *string `json:"full_name,omitempty"`
	Email          string  `json:"email"`
	ExistingStatus *string `json:"existing_status,omitempty"`
}

type CandidateListResponse struct {
	Candidates []CandidateResponse `json:"candidates"`
	Count      int                 `json:"count"`
}

// ParticipantDecisionRequest is one element of the bare JSON array body
// for PUT /api/admin/rounds/{id}/participants.
type ParticipantDecisionRequest struct {
	UserID string `json:"user_id"`
	Status string `json:"status"`
}
