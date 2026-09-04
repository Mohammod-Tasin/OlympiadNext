package dto

// UploadFileResponse is returned by POST /api/user/upload-file; the URL is
// what the caller then submits as verification_doc / profile_picture.
type UploadFileResponse struct {
	URL string `json:"url"`
}

// SubmitProfileRequest backs PUT /api/user/profile for both first-time
// onboarding and later profile edits: academic details plus optional KYC
// file references. verification_doc is optional — omit it to keep the
// document (and verification status) already on file; sending a new one
// moves the account back to 'pending' review.
type SubmitProfileRequest struct {
	FullName        string `json:"full_name"`
	InstitutionName string `json:"institution_name"`
	Level           string `json:"level"`
	Medium          string `json:"medium"`
	VerificationDoc string `json:"verification_doc,omitempty"`
	ProfilePicture  string `json:"profile_picture,omitempty"`
}
