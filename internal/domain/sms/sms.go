// Package sms abstracts outbound transactional SMS so the application
// layer never depends on a concrete provider such as BulkSMSBD.
package sms

import (
	"context"
	"errors"
)

type Sender interface {
	// Send delivers message to a Bangladeshi mobile number in local
	// 01XXXXXXXXX form; the adapter normalises it for the provider. A
	// send that could not be handed off wraps ErrDeliveryFailed (or
	// ErrNotConfigured when no credentials are set), so a best-effort
	// caller can carry on.
	Send(ctx context.Context, toPhone, message string) error
	// SendOTP delivers a verification code, mirroring email.Sender.SendOTP.
	SendOTP(ctx context.Context, toPhone, code string) error
}

// ErrNotConfigured marks a send attempted with no provider credentials.
// Startup does not fail when the SMS keys are unset (SMS is optional); the
// error surfaces only here, on the first real send. Callers test for it
// with errors.Is.
var ErrNotConfigured = errors.New("sms: provider not configured")

// ErrDeliveryFailed marks an SMS that could not be delivered — a network
// failure, a timeout, or a provider-side rejection. Callers that only need
// best-effort delivery log it and continue.
var ErrDeliveryFailed = errors.New("sms: delivery failed")
