package dto

import "time"

// PrizeRequest is the admin-supplied payload for both create and update.
// PrizeDescription is optional. Level must be one of "Junior",
// "Secondary", "Higher Secondary"; overlap validation is scoped to
// (event, level), so two levels may reuse the same rank range.
type PrizeRequest struct {
	RankFrom         int     `json:"rank_from"`
	RankTo           int     `json:"rank_to"`
	PrizeName        string  `json:"prize_name"`
	PrizeDescription *string `json:"prize_description,omitempty"`
	Level            string  `json:"level"`
}

// PrizeResponse is a prize tier as seen by both the admin console and the
// client frontend — there is no draft/active state to hide from either.
type PrizeResponse struct {
	ID               string    `json:"id"`
	EventID          string    `json:"event_id"`
	RankFrom         int       `json:"rank_from"`
	RankTo           int       `json:"rank_to"`
	PrizeName        string    `json:"prize_name"`
	PrizeDescription *string   `json:"prize_description,omitempty"`
	Level            string    `json:"level"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PrizeListResponse struct {
	Prizes []PrizeResponse `json:"prizes"`
	Count  int             `json:"count"`
}
