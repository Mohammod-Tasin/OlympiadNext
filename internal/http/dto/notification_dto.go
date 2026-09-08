package dto

// NotificationPreferenceRequest backs PUT /api/user/notification-preference.
// method is "email" or "phone"; phone is required (and only used) when
// method is "phone".
type NotificationPreferenceRequest struct {
	Method string `json:"method"`
	Phone  string `json:"phone,omitempty"`
}

// VerifyNotificationPhoneOTPRequest backs
// POST /api/user/notification-phone/verify-otp.
type VerifyNotificationPhoneOTPRequest struct {
	OTP string `json:"otp"`
}
