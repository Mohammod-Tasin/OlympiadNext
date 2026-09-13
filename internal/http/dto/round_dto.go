package dto

import "time"

// RoundRequest is the admin-supplied payload for both create and update.
// StartAt is an RFC3339 string, parsed strictly by the handler like
// EventRequest.EventDate. IsFinal marks this as the event's one final
// round; the backend rejects a second one for the same event.
type RoundRequest struct {
	RoundOrder      int    `json:"round_order"`
	RoundName       string `json:"round_name"`
	StartAt         string `json:"start_at"`
	DurationMinutes int    `json:"duration_minutes"`
	IsFinal         bool   `json:"is_final"`
}

// RoundResponse is a round as seen by both the admin console and the
// client frontend. YourStatus and Rank are only populated on the public
// listing, and only when the request carried a valid access token; Rank
// is further only ever non-nil when YourStatus is "winner".
type RoundResponse struct {
	ID              string    `json:"id"`
	EventID         string    `json:"event_id"`
	RoundOrder      int       `json:"round_order"`
	RoundName       string    `json:"round_name"`
	StartAt         time.Time `json:"start_at"`
	DurationMinutes int       `json:"duration_minutes"`
	Status          string    `json:"status"`
	IsFinal         bool      `json:"is_final"`
	YourStatus      *string   `json:"your_status,omitempty"`
	Rank            *int      `json:"rank,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RoundListResponse struct {
	Rounds []RoundResponse `json:"rounds"`
	Count  int             `json:"count"`
}

// CandidateResponse is one student eligible for a round's next decision.
// ExistingStatus is set when the admin has already decided this student
// in this round, so a UI can pre-tick it; ExistingRank is set alongside
// it only when that decision was "winner", so a UI can pre-fill rank too.
type CandidateResponse struct {
	UserID         string  `json:"user_id"`
	FullName       *string `json:"full_name,omitempty"`
	Email          string  `json:"email"`
	ExistingStatus *string `json:"existing_status,omitempty"`
	ExistingRank   *int    `json:"existing_rank,omitempty"`
}

type CandidateListResponse struct {
	Candidates []CandidateResponse `json:"candidates"`
	Count      int                 `json:"count"`
}

// ParticipantDecisionRequest is one element of the bare JSON array body
// for PUT /api/admin/rounds/{id}/participants. Rank is required when
// Status is "winner" and rejected otherwise.
type ParticipantDecisionRequest struct {
	UserID string `json:"user_id"`
	Status string `json:"status"`
	Rank   *int   `json:"rank,omitempty"`
}
