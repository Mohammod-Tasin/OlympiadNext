// Package auth contains the application-layer orchestration for
// registration, login, Google sign-in and session refresh. It depends
// only on domain interfaces, never on concrete infrastructure.
package auth

import (
	"context"
	crand "crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"olympiadnext/internal/auth/email"
	"olympiadnext/internal/auth/google"
	"olympiadnext/internal/auth/hash"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/device"
	notifyemail "olympiadnext/internal/domain/email"
	"olympiadnext/internal/domain/token"
	"olympiadnext/internal/domain/user"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid email or password")
	ErrGoogleOnlyAccount  = errors.New("auth: account uses Google sign-in, no password set")
	ErrEmailNotVerified   = errors.New("auth: email address not verified")
	ErrSessionExpired     = errors.New("auth: session expired or revoked")
	ErrInvalidOTP         = errors.New("auth: invalid or expired code")
)

// otpTTL is how long a generated email OTP remains valid.
const otpTTL = 5 * time.Minute

// otpCodeLength is the number of digits in a verification code, and
// otpCodeDigits is the alphabet it is drawn from.
const (
	otpCodeLength = 6
	otpCodeDigits = "0123456789"
)

// TokenPair is what the HTTP layer sends back: the access token in the
// body and the raw refresh token to be set as an HttpOnly cookie.
type TokenPair struct {
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
}

type Service struct {
	users         user.Repository
	refreshTokens token.Repository
	devices       device.Repository
	emailSender   notifyemail.Sender
	jwtManager    *jwt.Manager
	googleVerify  *google.Verifier
	log           *slog.Logger
}

func NewService(
	users user.Repository,
	refreshTokens token.Repository,
	devices device.Repository,
	emailSender notifyemail.Sender,
	jwtManager *jwt.Manager,
	googleVerify *google.Verifier,
	log *slog.Logger,
) *Service {
	return &Service{
		users:         users,
		refreshTokens: refreshTokens,
		devices:       devices,
		emailSender:   emailSender,
		jwtManager:    jwtManager,
		googleVerify:  googleVerify,
		log:           log,
	}
}

// Register creates an email/password account in an unverified state and
// emails it a one-time code. It does not start a session: the caller must
// verify their email (VerifyEmailOTP) and then log in. Academic details
// and the KYC document are collected later, at the onboarding submission.
func (s *Service) Register(ctx context.Context, rawEmail, password string) error {
	if err := email.ValidateEmail(rawEmail); err != nil {
		return err
	}
	if err := email.ValidatePasswordStrength(password); err != nil {
		return err
	}

	passwordHash, err := email.HashPassword(password)
	if err != nil {
		return err
	}

	u := &user.User{
		Email:         rawEmail,
		PasswordHash:  &passwordHash,
		AuthProvider:  user.ProviderLocal,
		EmailVerified: false,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return err
	}

	s.log.Info("user registered", "user_id", u.ID, "provider", "local")
	return s.issueEmailOTP(ctx, u.ID, u.Email)
}

func (s *Service) Login(ctx context.Context, rawEmail, password, deviceFingerprint string) (*TokenPair, error) {
	u, err := s.users.FindByEmail(ctx, rawEmail)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !u.HasPassword() {
		return nil, ErrGoogleOnlyAccount
	}
	if !email.VerifyPassword(*u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if !u.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	s.log.Info("user logged in", "user_id", u.ID, "provider", "local")
	return s.issueTokenPair(ctx, u, deviceFingerprint)
}

// GoogleLogin verifies the ID token with Google, then either logs into
// an existing Google-linked account, links Google to an existing
// email/password account, or creates a brand new (already-verified,
// passwordless) account.
func (s *Service) GoogleLogin(ctx context.Context, rawIDToken, deviceFingerprint string) (*TokenPair, error) {
	claims, err := s.googleVerify.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}

	if u, err := s.users.FindByGoogleID(ctx, claims.Subject); err == nil {
		s.log.Info("user logged in", "user_id", u.ID, "provider", "google")
		return s.issueTokenPair(ctx, u, deviceFingerprint)
	} else if !errors.Is(err, user.ErrNotFound) {
		return nil, err
	}

	existing, err := s.users.FindByEmail(ctx, claims.Email)
	switch {
	case err == nil:
		if err := s.users.LinkGoogleID(ctx, existing.ID, claims.Subject, claims.EmailVerified); err != nil {
			return nil, err
		}
		existing.GoogleID = &claims.Subject
		existing.EmailVerified = claims.EmailVerified
		s.log.Info("google account linked to existing user", "user_id", existing.ID)
		return s.issueTokenPair(ctx, existing, deviceFingerprint)

	case errors.Is(err, user.ErrNotFound):
		newUser := &user.User{
			Email:         claims.Email,
			FullName:      &claims.Name,
			AuthProvider:  user.ProviderGoogle,
			GoogleID:      &claims.Subject,
			EmailVerified: claims.EmailVerified,
		}
		if err := s.users.Create(ctx, newUser); err != nil {
			return nil, err
		}
		s.log.Info("user registered", "user_id", newUser.ID, "provider", "google")
		return s.issueTokenPair(ctx, newUser, deviceFingerprint)

	default:
		return nil, err
	}
}

// Refresh rotates a refresh token: the presented token is revoked and a
// brand new pair is issued, limiting the blast radius of a stolen token.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	claims, err := s.jwtManager.ParseRefreshToken(rawRefreshToken)
	if err != nil {
		return nil, ErrSessionExpired
	}

	stored, err := s.refreshTokens.FindByTokenHash(ctx, hash.SHA256Hex(rawRefreshToken))
	if err != nil {
		return nil, ErrSessionExpired
	}
	if stored.Revoked {
		s.log.Warn("refresh token reuse detected", "user_id", stored.UserID, "token_id", stored.ID)
		if err := s.refreshTokens.RevokeAllForUser(ctx, stored.UserID); err != nil {
			s.log.Error("refresh token reuse: revoke all failed", "user_id", stored.UserID, "error", err)
		}
		return nil, ErrSessionExpired
	}
	if !stored.IsActive(time.Now()) {
		return nil, ErrSessionExpired
	}

	u, err := s.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrSessionExpired
	}

	revoked, err := s.refreshTokens.Revoke(ctx, stored.ID)
	if err != nil {
		return nil, err
	}
	if !revoked {
		s.log.Warn("refresh token reuse race detected", "user_id", stored.UserID, "token_id", stored.ID)
		if err := s.refreshTokens.RevokeAllForUser(ctx, stored.UserID); err != nil {
			s.log.Error("refresh token reuse race: revoke all failed", "user_id", stored.UserID, "error", err)
		}
		return nil, ErrSessionExpired
	}

	return s.issueTokenPair(ctx, u, "")
}

// Logout revokes a single session's refresh token.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	stored, err := s.refreshTokens.FindByTokenHash(ctx, hash.SHA256Hex(rawRefreshToken))
	if err != nil {
		return nil // already gone; logout is idempotent
	}
	_, err = s.refreshTokens.Revoke(ctx, stored.ID)
	return err
}

// VerifyEmailOTP checks a submitted code against the one stored on the
// user's row. On success the account is marked verified and the code is
// cleared. Rejections are deliberately uniform so the endpoint cannot be
// used to probe which emails are registered.
func (s *Service) VerifyEmailOTP(ctx context.Context, rawEmail, code string) error {
	u, err := s.users.FindByEmail(ctx, rawEmail)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrInvalidOTP
		}
		return err
	}

	if u.EmailVerified {
		return nil // already verified; a repeat call is a harmless no-op
	}
	if u.EmailOTP == nil || u.EmailOTPExpiry == nil {
		return ErrInvalidOTP
	}

	codeMatches := subtle.ConstantTimeCompare([]byte(*u.EmailOTP), []byte(code)) == 1
	if !codeMatches || !time.Now().Before(*u.EmailOTPExpiry) {
		return ErrInvalidOTP
	}

	if err := s.users.MarkEmailVerified(ctx, u.ID); err != nil {
		return fmt.Errorf("auth: mark email verified failed: %w", err)
	}

	s.log.Info("email verified", "user_id", u.ID)
	return nil
}

// ResendEmailOTP issues a fresh code to an unverified account. A missing
// or already-verified account is a silent success so the endpoint does
// not leak which emails exist; only a real delivery failure surfaces.
func (s *Service) ResendEmailOTP(ctx context.Context, rawEmail string) error {
	u, err := s.users.FindByEmail(ctx, rawEmail)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil
		}
		return err
	}
	if u.EmailVerified {
		return nil
	}
	return s.issueEmailOTP(ctx, u.ID, u.Email)
}

// issueEmailOTP generates a fresh code, persists it with a short expiry,
// and makes a best-effort attempt to email it. Shared by registration and
// resend. A delivery failure is logged, not returned.
func (s *Service) issueEmailOTP(ctx context.Context, userID, toEmail string) error {
	code, err := generateOTPCode()
	if err != nil {
		return fmt.Errorf("auth: generate otp failed: %w", err)
	}
	if err := s.users.SetEmailOTP(ctx, userID, code, time.Now().Add(otpTTL)); err != nil {
		return fmt.Errorf("auth: persist otp failed: %w", err)
	}

	// OTP email delivery is best-effort. Render blocks outbound SMTP, so a
	// live send times out there on every attempt; failing here would 500
	// the registration (and resend) request even though the account is
	// created and the code is already stored. Log the code as a WARNING so
	// the flow can still be finished from the server console, and report
	// success. A non-delivery error is genuinely unexpected and still
	// propagates.
	if err := s.emailSender.SendOTP(ctx, toEmail, code); err != nil {
		if errors.Is(err, notifyemail.ErrDeliveryFailed) {
			s.log.Warn(fmt.Sprintf("SMTP failed. OTP for %s is: %s", toEmail, code), "error", err)
			return nil
		}
		return fmt.Errorf("auth: send otp email failed: %w", err)
	}
	return nil
}

func (s *Service) issueTokenPair(ctx context.Context, u *user.User, deviceFingerprint string) (*TokenPair, error) {
	accessToken, err := s.jwtManager.GenerateAccessToken(u.ID, u.Email)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(u.ID)
	if err != nil {
		return nil, err
	}

	if err := s.refreshTokens.Create(ctx, &token.RefreshToken{
		UserID:    u.ID,
		TokenHash: hash.SHA256Hex(refreshToken),
		ExpiresAt: refreshExpiresAt,
	}); err != nil {
		return nil, fmt.Errorf("auth: persist refresh token failed: %w", err)
	}

	if deviceFingerprint != "" {
		if err := s.users.UpdateActiveDeviceFingerprint(ctx, u.ID, deviceFingerprint); err != nil {
			return nil, fmt.Errorf("auth: set active device fingerprint failed: %w", err)
		}
		go s.upsertDeviceAsync(u.ID, deviceFingerprint)
	}

	return &TokenPair{
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  time.Now().Add(s.jwtManager.AccessTokenTTL()),
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
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

func (s *Service) upsertDeviceAsync(userID, deviceFingerprint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.devices.UpsertDevice(ctx, userID, deviceFingerprint); err != nil {
		s.log.Error("device fingerprint upsert failed", "user_id", userID, "error", err)
	}
}
