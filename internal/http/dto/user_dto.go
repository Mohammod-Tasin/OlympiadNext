package dto

// UploadFileResponse is returned by POST /api/user/upload-file; the URL is
// what the caller then submits as verification_doc / profile_picture.
type UploadFileResponse struct {
	URL string `json:"url"`
}

// SubmitProfileRequest is the onboarding submission: academic details plus
// the uploaded KYC file references. verification_doc is required; a
// successful submission moves the account to 'pending' review.
type SubmitProfileRequest struct {
	FullName        string `json:"full_name"`
	InstitutionName string `json:"institution_name"`
	Level           string `json:"level"`
	Medium          string `json:"medium"`
	VerificationDoc string `json:"verification_doc"`
	ProfilePicture  string `json:"profile_picture,omitempty"`
}
