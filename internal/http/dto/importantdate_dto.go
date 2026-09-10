package dto

import "time"

// ImportantDateRequest is the admin-supplied payload for both create and
// update. EventDate is received as a "2006-01-02" date string and parsed
// strictly by the handler, so a malformed value yields a clear 400 rather
// than a generic "invalid request body". IsActive is a pointer so an
// omitted field on create falls back to the column default (true) rather
// than to Go's zero value (false); on update an omitted field is likewise
// treated as "leave active".
type ImportantDateRequest struct {
	EventDate    string `json:"event_date"`
	Title        string `json:"title"`
	DetailsEN    string `json:"details_en"`
	DetailsBN    string `json:"details_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

// ActiveOrDefault resolves the optional is_active flag: true when the
// caller omitted it, otherwise the value they sent.
func (r ImportantDateRequest) ActiveOrDefault() bool {
	if r.IsActive == nil {
		return true
	}
	return *r.IsActive
}

type ImportantDateResponse struct {
	ID           string    `json:"id"`
	EventDate    string    `json:"event_date"`
	Title        string    `json:"title"`
	DetailsEN    string    `json:"details_en"`
	DetailsBN    string    `json:"details_bn"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ImportantDateListResponse struct {
	ImportantDates []ImportantDateResponse `json:"important_dates"`
	Count          int                     `json:"count"`
}
