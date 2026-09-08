// Package notify contains the application-layer orchestration for a user's
// transactional-notification preference: choosing email or SMS, proving a
// phone number with an OTP, and dispatching a notification on the channel
// the user settled on. It depends only on domain interfaces.
//
// The phone-OTP flow mirrors the email-verification OTP in package auth
// exactly: a crypto/rand 6-digit code, a 5-minute TTL, a constant-time
// compare, and one UPDATE that both verifies and nullifies the code so it
// cannot be replayed.
package notify

import (
	"context"
	crand "crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"

	emaildomain "olympiadnext/internal/domain/email"
	smsdomain "olympiadnext/internal/domain/sms"
	"olympiadnext/internal/domain/user"
)

var (
	// ErrValidation is a malformed preference change (unknown method, bad
	// phone number). Handlers map it to 400.
	ErrValidation = errors.New("notify: invalid request")
	// ErrInvalidOTP is a wrong or expired phone verification code.
	ErrInvalidOTP = errors.New("notify: invalid or expired code")
	// ErrNoPendingPhone is a verify/resend with no phone number on file to
	// act on.
	ErrNoPendingPhone = errors.New("notify: no notification phone awaiting verification")
)

// otpTTL, otpCodeLength and otpCodeDigits mirror the email OTP constants
// in package auth.
const otpTTL = 5 * time.Minute

const (
	otpCodeLength = 6
	otpCodeDigits = "0123456789"
)

// A Bangladeshi mobile number in local form, matching the one the exam
// registration flow accepts.
var phonePattern = regexp.MustCompile(`^01[3-9][0-9]{8}$`)

type Service struct {
	users user.Repository
	email emaildomain.Sender
	sms   smsdomain.Sender
	log   *slog.Logger
}

func NewService(users user.Repository, email emaildomain.Sender, sms smsdomain.Sender, log *slog.Logger) *Service {
	return &Service{users: users, email: email, sms: sms, log: log}
}

// SetPreference applies PUT /api/user/notification-preference. Choosing
// 'email' takes effect immediately. Choosing 'phone' stores the number and
// sends an OTP but does NOT switch the active channel — that happens only
// once VerifyPhoneOTP succeeds. pendingVerification reports which case ran.
func (s *Service) SetPreference(ctx context.Context, userID, method, phone string) (pendingVerification bool, err error) {
	switch user.NotificationMethod(strings.ToLower(strings.TrimSpace(method))) {
	case user.NotificationEmail:
		if err := s.users.SetNotificationMethod(ctx, userID, user.NotificationEmail); err != nil {
			return false, err
		}
		s.log.Info("notification method set to email", "user_id", userID)
		return false, nil

	case user.NotificationPhone:
		normalized, ok := normalizePhone(phone)
		if !ok {
			return false, fmt.Errorf("%w: phone must be a valid Bangladeshi mobile number", ErrValidation)
		}
		if err := s.issuePhoneOTP(ctx, userID, normalized); err != nil {
			return false, err
		}
		return true, nil

	default:
		return false, fmt.Errorf("%w: method must be 'email' or 'phone'", ErrValidation)
	}
}

// VerifyPhoneOTP consumes the code from POST
// /api/user/notification-phone/verify-otp. On success the number is marked
// verified and the active channel moves to 'phone'.
func (s *Service) VerifyPhoneOTP(ctx context.Context, userID, code string) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.NotificationPhone == nil {
		return ErrNoPendingPhone
	}
	if u.NotificationPhoneVerified && u.NotificationMethod == user.NotificationPhone {
		return nil // already done; a repeat call is a harmless no-op
	}
	if u.NotificationPhoneOTP == nil || u.NotificationPhoneOTPExpiry == nil {
		return ErrInvalidOTP
	}

	matches := subtle.ConstantTimeCompare([]byte(*u.NotificationPhoneOTP), []byte(strings.TrimSpace(code))) == 1
	if !matches || !time.Now().Before(*u.NotificationPhoneOTPExpiry) {
		return ErrInvalidOTP
	}

	if err := s.users.MarkNotificationPhoneVerified(ctx, userID); err != nil {
		return fmt.Errorf("notify: mark phone verified failed: %w", err)
	}
	s.log.Info("notification phone verified", "user_id", userID)
	return nil
}

// ResendPhoneOTP re-issues a code to the number already on file, for
// POST /api/user/notification-phone/resend-otp.
func (s *Service) ResendPhoneOTP(ctx context.Context, userID string) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.NotificationPhone == nil {
		return ErrNoPendingPhone
	}
	return s.issuePhoneOTP(ctx, userID, *u.NotificationPhone)
}

// issuePhoneOTP generates a fresh code, persists it, and makes a
// best-effort attempt to text it. A delivery failure (including "SMS not
// configured") is logged, not returned — exactly as issueEmailOTP treats a
// failed SMTP send — so the request still succeeds and the code can be
// recovered from the logs in development.
func (s *Service) issuePhoneOTP(ctx context.Context, userID, phone string) error {
	code, err := generateOTPCode()
	if err != nil {
		return fmt.Errorf("notify: generate otp failed: %w", err)
	}
	if err := s.users.SetNotificationPhoneOTP(ctx, userID, phone, code, time.Now().Add(otpTTL)); err != nil {
		return fmt.Errorf("notify: persist otp failed: %w", err)
	}

	if err := s.sms.SendOTP(ctx, phone, code); err != nil {
		if errors.Is(err, smsdomain.ErrNotConfigured) || errors.Is(err, smsdomain.ErrDeliveryFailed) {
			s.log.Warn(fmt.Sprintf("SMS failed. Notification phone OTP for %s is: %s", phone, code), "user_id", userID, "error", err)
			return nil
		}
		return fmt.Errorf("notify: send otp sms failed: %w", err)
	}
	s.log.Info("notification phone otp sent", "user_id", userID)
	return nil
}

// NotifyAdmitCardReady dispatches the "your admit card is ready" alert on
// the user's chosen channel: SMS when they verified a phone and selected
// it, email otherwise. Delivery is best-effort — a failure is logged at
// WARN/ERROR and swallowed, since the admit card itself is already stored
// and downloadable.
func (s *Service) NotifyAdmitCardReady(ctx context.Context, userID, eventTitle string) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		s.log.Error("admit-card notification: user lookup failed", "user_id", userID, "error", err)
		return
	}

	event := strings.TrimSpace(eventTitle)
	if event == "" {
		event = "your exam"
	}

	usePhone := u.NotificationMethod == user.NotificationPhone &&
		u.NotificationPhoneVerified &&
		u.NotificationPhone != nil

	if usePhone {
		msg := fmt.Sprintf("Shikhor: your admit card for %s is ready. Log in to download it.", event)
		if err := s.sms.Send(ctx, *u.NotificationPhone, msg); err != nil {
			s.log.Warn("admit-card notification: sms delivery failed", "user_id", userID, "error", err)
		} else {
			s.log.Info("admit-card notification sent", "user_id", userID, "channel", "sms")
		}
		return
	}

	subject := "Your Shikhor admit card is ready"
	body := fmt.Sprintf("Your admit card for %s is ready. Log in to your Shikhor account to download it.", event)
	if err := s.email.Send(ctx, u.Email, subject, body); err != nil {
		s.log.Warn("admit-card notification: email delivery failed", "user_id", userID, "error", err)
		return
	}
	s.log.Info("admit-card notification sent", "user_id", userID, "channel", "email")
}

func generateOTPCode() (string, error) {
	code := make([]byte, otpCodeLength)
	for i := range code {
		n, err := crand.Int(crand.Reader, big.NewInt(int64(len(otpCodeDigits))))
		if err != nil {
			return "", err
		}
		code[i] = otpCodeDigits[n.Int64()]
	}
	return string(code), nil
}

// normalizePhone accepts 01XXXXXXXXX, +8801XXXXXXXXX or 8801XXXXXXXXX
// (with spaces/dashes) and returns the canonical 01XXXXXXXXX form.
func normalizePhone(raw string) (string, bool) {
	n := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	n = strings.TrimPrefix(n, "+")
	if strings.HasPrefix(n, "880") {
		n = "0" + strings.TrimPrefix(n, "880")
	}
	if !phonePattern.MatchString(n) {
		return "", false
	}
	return n, true
}
