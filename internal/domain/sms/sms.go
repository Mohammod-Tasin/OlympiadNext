// Package sms abstracts outbound SMS delivery so the application layer
// never depends on a concrete provider such as BulkSMSBD.
package sms

import "context"

type Sender interface {
	SendSMS(ctx context.Context, number, message string) error
}
