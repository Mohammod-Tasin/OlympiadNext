package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	smsdomain "olympiadnext/internal/domain/sms"
	"olympiadnext/internal/domain/user"
)

// --- test doubles -----------------------------------------------------------

// fakeUserRepo embeds user.Repository, so any method a test path does not
// use stays nil and panics loudly if it is ever called.
type fakeUserRepo struct {
	user.Repository
	findByID               func(ctx context.Context, id string) (*user.User, error)
	setMethod              func(ctx context.Context, id string, m user.NotificationMethod) error
	setPhoneOTP            func(ctx context.Context, id, phone, code string, exp time.Time) error
	markPhoneVerified      func(ctx context.Context, id string) error
	markPhoneVerifiedCalls int
	setPhoneOTPLastCode    string
	setPhoneOTPLastPhone   string
}

func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*user.User, error) {
	return f.findByID(ctx, id)
}
func (f *fakeUserRepo) SetNotificationMethod(ctx context.Context, id string, m user.NotificationMethod) error {
	return f.setMethod(ctx, id, m)
}
func (f *fakeUserRepo) SetNotificationPhoneOTP(ctx context.Context, id, phone, code string, exp time.Time) error {
	f.setPhoneOTPLastCode = code
	f.setPhoneOTPLastPhone = phone
	if f.setPhoneOTP != nil {
		return f.setPhoneOTP(ctx, id, phone, code, exp)
	}
	return nil
}
func (f *fakeUserRepo) MarkNotificationPhoneVerified(ctx context.Context, id string) error {
	f.markPhoneVerifiedCalls++
	if f.markPhoneVerified != nil {
		return f.markPhoneVerified(ctx, id)
	}
	return nil
}

// mockSMS records what it was asked to send and never calls a live API.
type mockSMS struct {
	sendErr    error
	otpErr     error
	sentMsgs   []string
	sentPhones []string
	otpCalls   int
}

func (m *mockSMS) Send(_ context.Context, toPhone, message string) error {
	m.sentPhones = append(m.sentPhones, toPhone)
	m.sentMsgs = append(m.sentMsgs, message)
	return m.sendErr
}
func (m *mockSMS) SendOTP(_ context.Context, toPhone, code string) error {
	m.otpCalls++
	m.sentPhones = append(m.sentPhones, toPhone)
	m.sentMsgs = append(m.sentMsgs, "OTP:"+code)
	return m.otpErr
}

// mockEmail records deliveries; SendOTP is unused here.
type mockEmail struct {
	sendErr   error
	sentTo    []string
	sentSubj  []string
	sendCalls int
}

func (m *mockEmail) SendOTP(_ context.Context, _, _ string) error { return nil }
func (m *mockEmail) Send(_ context.Context, to, subject, _ string) error {
	m.sendCalls++
	m.sentTo = append(m.sentTo, to)
	m.sentSubj = append(m.sentSubj, subject)
	return m.sendErr
}

func newService(users user.Repository, sms smsdomain.Sender, email *mockEmail) *Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(users, email, sms, log)
}

func ptr(s string) *string { return &s }

// --- SetPreference --------------------------------------------------------

func TestSetPreference_Email_TakesEffectImmediately(t *testing.T) {
	var gotMethod user.NotificationMethod
	users := &fakeUserRepo{setMethod: func(_ context.Context, _ string, m user.NotificationMethod) error {
		gotMethod = m
		return nil
	}}
	svc := newService(users, &mockSMS{}, &mockEmail{})

	pending, err := svc.SetPreference(context.Background(), "u1", "email", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending {
		t.Fatal("email preference must not require verification")
	}
	if gotMethod != user.NotificationEmail {
		t.Fatalf("method = %q, want email", gotMethod)
	}
}

func TestSetPreference_Phone_StoresOTPButDoesNotSwitchChannel(t *testing.T) {
	users := &fakeUserRepo{
		setMethod: func(_ context.Context, _ string, _ user.NotificationMethod) error {
			t.Fatal("notification_method must not change before the phone is verified")
			return nil
		},
	}
	sms := &mockSMS{}
	svc := newService(users, sms, &mockEmail{})

	pending, err := svc.SetPreference(context.Background(), "u1", "phone", "+8801712345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pending {
		t.Fatal("phone preference must report pending verification")
	}
	if users.setPhoneOTPLastPhone != "01712345678" {
		t.Fatalf("phone not normalized before persist: %q", users.setPhoneOTPLastPhone)
	}
	if sms.otpCalls != 1 {
		t.Fatalf("expected one OTP SMS, got %d", sms.otpCalls)
	}
	if len(users.setPhoneOTPLastCode) != otpCodeLength {
		t.Fatalf("OTP code length = %d, want %d", len(users.setPhoneOTPLastCode), otpCodeLength)
	}
}

func TestSetPreference_Phone_InvalidNumber(t *testing.T) {
	svc := newService(&fakeUserRepo{}, &mockSMS{}, &mockEmail{})
	if _, err := svc.SetPreference(context.Background(), "u1", "phone", "12345"); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestSetPreference_UnknownMethod(t *testing.T) {
	svc := newService(&fakeUserRepo{}, &mockSMS{}, &mockEmail{})
	if _, err := svc.SetPreference(context.Background(), "u1", "carrier-pigeon", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestSetPreference_Phone_SMSFailureIsSwallowed(t *testing.T) {
	// Mirrors issueEmailOTP: a delivery failure logs the code and still
	// succeeds so the user can recover it from the logs.
	users := &fakeUserRepo{}
	sms := &mockSMS{otpErr: smsdomain.ErrNotConfigured}
	svc := newService(users, sms, &mockEmail{})

	pending, err := svc.SetPreference(context.Background(), "u1", "phone", "01712345678")
	if err != nil {
		t.Fatalf("a best-effort SMS failure must not fail the request: %v", err)
	}
	if !pending {
		t.Fatal("expected pending verification")
	}
	if users.setPhoneOTPLastCode == "" {
		t.Fatal("code should still have been persisted")
	}
}

// --- VerifyPhoneOTP: success / fail / expiry ------------------------------

func userWithPhoneOTP(code string, expiry time.Time) *user.User {
	return &user.User{
		ID:                         "u1",
		Email:                      "student@example.com",
		NotificationMethod:         user.NotificationEmail,
		NotificationPhone:          ptr("01712345678"),
		NotificationPhoneOTP:       ptr(code),
		NotificationPhoneOTPExpiry: &expiry,
	}
}

func TestVerifyPhoneOTP_Success(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return userWithPhoneOTP("123456", time.Now().Add(2*time.Minute)), nil
	}}
	svc := newService(users, &mockSMS{}, &mockEmail{})

	if err := svc.VerifyPhoneOTP(context.Background(), "u1", "123456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if users.markPhoneVerifiedCalls != 1 {
		t.Fatalf("expected MarkNotificationPhoneVerified once, got %d", users.markPhoneVerifiedCalls)
	}
}

func TestVerifyPhoneOTP_WrongCode(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return userWithPhoneOTP("123456", time.Now().Add(2*time.Minute)), nil
	}}
	svc := newService(users, &mockSMS{}, &mockEmail{})

	if err := svc.VerifyPhoneOTP(context.Background(), "u1", "000000"); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("want ErrInvalidOTP, got %v", err)
	}
	if users.markPhoneVerifiedCalls != 0 {
		t.Fatal("a wrong code must not verify the phone")
	}
}

func TestVerifyPhoneOTP_Expired(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return userWithPhoneOTP("123456", time.Now().Add(-1*time.Minute)), nil
	}}
	svc := newService(users, &mockSMS{}, &mockEmail{})

	if err := svc.VerifyPhoneOTP(context.Background(), "u1", "123456"); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("want ErrInvalidOTP for an expired code, got %v", err)
	}
	if users.markPhoneVerifiedCalls != 0 {
		t.Fatal("an expired code must not verify the phone")
	}
}

func TestVerifyPhoneOTP_NoPendingPhone(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return &user.User{ID: "u1", NotificationMethod: user.NotificationEmail}, nil
	}}
	svc := newService(users, &mockSMS{}, &mockEmail{})

	if err := svc.VerifyPhoneOTP(context.Background(), "u1", "123456"); !errors.Is(err, ErrNoPendingPhone) {
		t.Fatalf("want ErrNoPendingPhone, got %v", err)
	}
}

// --- NotifyAdmitCardReady: channel routing ------------------------------

func TestNotifyAdmitCardReady_RoutesToSMSWhenPhoneVerified(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return &user.User{
			ID:                        "u1",
			Email:                     "student@example.com",
			NotificationMethod:        user.NotificationPhone,
			NotificationPhone:         ptr("01712345678"),
			NotificationPhoneVerified: true,
		}, nil
	}}
	sms := &mockSMS{}
	email := &mockEmail{}
	svc := newService(users, sms, email)

	svc.NotifyAdmitCardReady(context.Background(), "u1", "Math Olympiad")

	if len(sms.sentMsgs) != 1 {
		t.Fatalf("expected one SMS, got %d", len(sms.sentMsgs))
	}
	if email.sendCalls != 0 {
		t.Fatal("email must not be used when the verified channel is SMS")
	}
}

func TestNotifyAdmitCardReady_RoutesToEmailByDefault(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return &user.User{
			ID:                 "u1",
			Email:              "student@example.com",
			NotificationMethod: user.NotificationEmail,
		}, nil
	}}
	sms := &mockSMS{}
	email := &mockEmail{}
	svc := newService(users, sms, email)

	svc.NotifyAdmitCardReady(context.Background(), "u1", "Math Olympiad")

	if email.sendCalls != 1 || len(email.sentTo) != 1 || email.sentTo[0] != "student@example.com" {
		t.Fatalf("expected one email to the student, got calls=%d to=%v", email.sendCalls, email.sentTo)
	}
	if len(sms.sentMsgs) != 0 {
		t.Fatal("SMS must not be used for an email-preference user")
	}
}

func TestNotifyAdmitCardReady_FallsBackToEmailWhenPhoneUnverified(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return &user.User{
			ID:                        "u1",
			Email:                     "student@example.com",
			NotificationMethod:        user.NotificationPhone, // set, but...
			NotificationPhone:         ptr("01712345678"),
			NotificationPhoneVerified: false, // ...never verified
		}, nil
	}}
	sms := &mockSMS{}
	email := &mockEmail{}
	svc := newService(users, sms, email)

	svc.NotifyAdmitCardReady(context.Background(), "u1", "")

	if email.sendCalls != 1 {
		t.Fatalf("expected email fallback, got %d email calls", email.sendCalls)
	}
	if len(sms.sentMsgs) != 0 {
		t.Fatal("an unverified phone must not receive SMS")
	}
}

func TestNotifyAdmitCardReady_SMSFailureIsSwallowed(t *testing.T) {
	users := &fakeUserRepo{findByID: func(_ context.Context, _ string) (*user.User, error) {
		return &user.User{
			ID:                        "u1",
			Email:                     "student@example.com",
			NotificationMethod:        user.NotificationPhone,
			NotificationPhone:         ptr("01712345678"),
			NotificationPhoneVerified: true,
		}, nil
	}}
	svc := newService(users, &mockSMS{sendErr: smsdomain.ErrDeliveryFailed}, &mockEmail{})

	// Must not panic or block; the admit card is already stored.
	svc.NotifyAdmitCardReady(context.Background(), "u1", "Math Olympiad")
}
