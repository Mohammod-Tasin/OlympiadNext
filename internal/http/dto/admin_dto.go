package dto

import "time"

// AdminUserResponse is a single row in the admin user list. It exposes the
// verification fields an admin needs to review an account, including the
// document URLs (fetchable by an admin through the gated /uploads route).
type AdminUserResponse struct {
	UserID             string    `json:"user_id"`
	Email              string    `json:"email"`
	FullName           *string   `json:"full_name,omitempty"`
	Role               string    `json:"role"`
	EmailVerified      bool      `json:"email_verified"`
	InstitutionName    *string   `json:"institution_name,omitempty"`
	Level              *string   `json:"level,omitempty"`
	Medium             *string   `json:"medium,omitempty"`
	ProfilePicture     *string   `json:"profile_picture,omitempty"`
	VerificationDoc    *string   `json:"verification_doc,omitempty"`
	VerificationStatus string    `json:"verification_status"`
	CreatedAt          time.Time `json:"created_at"`
}

type AdminUserListResponse struct {
	Users []AdminUserResponse `json:"users"`
	Count int                 `json:"count"`
}

// VerifyUserRequest is the admin's review decision for
// PUT /api/admin/users/{id}/verify.
type VerifyUserRequest struct {
	Status string `json:"status"`
}
