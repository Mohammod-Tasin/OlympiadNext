package dto

import "time"

// NoticeRequest is the admin-supplied payload for both create and update.
// IsActive is a pointer so an omitted field on create falls back to the
// column default (true) rather than to Go's zero value (false); on
// update an omitted field is likewise treated as "leave active".
type NoticeRequest struct {
	TextEN       string `json:"text_en"`
	TextBN       string `json:"text_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

// ActiveOrDefault resolves the optional is_active flag: true when the
// caller omitted it, otherwise the value they sent.
func (r NoticeRequest) ActiveOrDefault() bool {
	if r.IsActive == nil {
		return true
	}
	return *r.IsActive
}

type NoticeResponse struct {
	ID           string    `json:"id"`
	TextEN       string    `json:"text_en"`
	TextBN       string    `json:"text_bn"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NoticeListResponse struct {
	Notices []NoticeResponse `json:"notices"`
	Count   int              `json:"count"`
}
