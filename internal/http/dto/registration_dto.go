package dto

import "time"

// CreateRegistrationRequest is the student-supplied payload for
// POST /api/user/registrations.
type CreateRegistrationRequest struct {
	EventID       string `json:"event_id"`
	PaymentMethod string `json:"payment_method"`
	SenderNumber  string `json:"sender_number"`
	TransactionID string `json:"transaction_id"`
}

// RegistrationResponse is one row of the student's own registration list
// (GET /api/user/registrations) and the body returned by a successful
// submission. It deliberately omits reviewer identity.
type RegistrationResponse struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	EventTitle    string     `json:"event_title,omitempty"`
	PaymentMethod string     `json:"payment_method"`
	SenderNumber  string     `json:"sender_number"`
	TransactionID string     `json:"transaction_id"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
}

type RegistrationListResponse struct {
	Registrations []RegistrationResponse `json:"registrations"`
	Count         int                    `json:"count"`
}

// AdminRegistrationResponse is one row of the admin review queue
// (GET /api/admin/registrations). It adds the student's identity to the
// fields an admin needs to match the payment against their statement.
type AdminRegistrationResponse struct {
	ID            string     `json:"id"`
	StudentName   *string    `json:"student_name,omitempty"`
	StudentEmail  string     `json:"student_email"`
	EventID       string     `json:"event_id"`
	EventTitle    string     `json:"event_title"`
	PaymentMethod string     `json:"payment_method"`
	SenderNumber  string     `json:"sender_number"`
	TransactionID string     `json:"transaction_id"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
}

type AdminRegistrationListResponse struct {
	Registrations []AdminRegistrationResponse `json:"registrations"`
	Count         int                         `json:"count"`
}

// ReviewRegistrationRequest is the admin's decision for
// PUT /api/admin/registrations/{id}/review.
type ReviewRegistrationRequest struct {
	Status string `json:"status"`
}
