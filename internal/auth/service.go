// Package auth contains the application-layer orchestration for
// registration, email verification, login, Google sign-in and session
// refresh. It depends only on domain interfaces, never on concrete
// infrastructure.
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
	ErrEmailNotVerified   = errors.New("auth: email address is not verified")
	ErrSessionExpired     = errors.New("auth: session expired or revoked")
	ErrInvalidOTP         = errors.New("auth: invalid or expired code")
	ErrAlreadyVerified    = errors.New("auth: email address is already verified")
)

// otpTTL is how long a generated email OTP remains valid.
const otpTTL = 5 * time.Minute

// otpCodeDigits is the alphabet the OTP generator draws from to build a
// 6-digit numeric code.
const otpCodeDigits = "0123456789"

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

// Register creates an unverified local account and mails it a 6-digit
// OTP. It deliberately returns no session: the caller must complete
// VerifyEmailOTP before Login will hand out tokens.
func (s *Service) Register(ctx context.Context, rawEmail, password, fullName, institutionName, level, medium string) error {
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
		Email:           rawEmail,
		FullName:        &fullName,
		PasswordHash:    &passwordHash,
		AuthProvider:    user.ProviderLocal,
		EmailVerified:   false,
		InstitutionName: &institutionName,
		Level:           &level,
		Medium:          &medium,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return err
	}

	s.log.Info("user registered", "user_id", u.ID, "provider", "local")
	return s.issueEmailOTP(ctx, u)
}

// SendEmailOTP re-issues a verification code, overwriting any code still
// outstanding. Callers are unauthenticated here (a user awaiting
// verification has no access token), so the HTTP layer must not leak
// whether the address exists.
func (s *Service) SendEmailOTP(ctx context.Context, rawEmail string) error {
	u, err := s.users.FindByEmail(ctx, rawEmail)
	if err != nil {
		return err
	}
	if u.EmailVerified {
		return ErrAlreadyVerified
	}
	return s.issueEmailOTP(ctx, u)
}

// VerifyEmailOTP consumes the outstanding code for the address. On
// success MarkEmailVerified both sets the flag and nullifies the code, so
// the same OTP cannot be replayed.
func (s *Service) VerifyEmailOTP(ctx context.Context, rawEmail, code string) error {
	u, err := s.users.FindByEmail(ctx, rawEmail)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrInvalidOTP
		}
		return err
	}
	if u.EmailVerified {
		return ErrAlreadyVerified
	}
	if u.EmailOTP == nil || u.EmailOTPExpiry == nil {
		return ErrInvalidOTP
	}

	// Constant-time compare so a timing side channel cannot be used to
	// recover the code digit by digit.
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

// Login checks the bcrypt hash and refuses accounts whose address has
// never been confirmed, so an unverified registration cannot be used.
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
// email/password account, or creates a brand new account. Google has
// already confirmed the address, so no OTP round-trip is needed.
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
		existing.EmailVerified = existing.EmailVerified || claims.EmailVerified
		s.log.Info("google account linked to existing user", "user_id", existing.ID)
		return s.issueTokenPair(ctx, existing, deviceFingerprint)

	case errors.Is(err, user.ErrNotFound):
		newUser := &user.User{
			Email:         claims.Email,
			FullName:      &claims.Name,
			AuthProvider:  user.ProviderGoogle,
			GoogleID:      &claims.Subject,
			EmailVerified: true,
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

// issueEmailOTP generates a fresh 6-digit code, persists it with a
// 5-minute expiry, and mails it over SMTP (or logs it in local dev when
// no SMTP credentials are configured).
func (s *Service) issueEmailOTP(ctx context.Context, u *user.User) error {
	code, err := generateOTPCode()
	if err != nil {
		return fmt.Errorf("auth: generate otp failed: %w", err)
	}

	if err := s.users.SetEmailOTP(ctx, u.ID, code, time.Now().Add(otpTTL)); err != nil {
		return fmt.Errorf("auth: persist otp failed: %w", err)
	}
	if err := s.emailSender.SendOTP(ctx, u.Email, code); err != nil {
		return fmt.Errorf("auth: send otp email failed: %w", err)
	}

	s.log.Info("email otp issued", "user_id", u.ID)
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
	code := make([]byte, 6)
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
