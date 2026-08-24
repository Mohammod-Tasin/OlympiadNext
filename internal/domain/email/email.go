// Package email abstracts outbound transactional email so the
// application layer never depends on a concrete provider such as SMTP.
package email

import "context"

type Sender interface {
	SendOTP(ctx context.Context, toEmail, code string) error
}
