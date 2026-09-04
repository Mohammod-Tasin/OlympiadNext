// Package email abstracts outbound transactional email so the
// application layer never depends on a concrete provider such as SMTP.
package email

import (
	"context"
	"errors"
)

type Sender interface {
	// SendOTP delivers a verification code to toEmail. When the code was
	// generated but could not be handed off for delivery (a timed-out or
	// refused SMTP send), the returned error wraps ErrDeliveryFailed so a
	// caller that only needs best-effort delivery can carry on.
	SendOTP(ctx context.Context, toEmail, code string) error
}

// ErrDeliveryFailed marks an OTP email that could not be delivered —
// typically because the host blocks outbound SMTP (Render blocks ports
// 465/587). Registration treats it as non-fatal: it logs the code for
// console recovery instead of failing the request. Callers test for it
// with errors.Is.
var ErrDeliveryFailed = errors.New("email: delivery failed")
