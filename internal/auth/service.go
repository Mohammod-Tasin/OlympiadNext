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
	"olympiadnext/internal/domain/otp"
	"olympiadnext/internal/domain/sms"
	"olympiadnext/internal/domain/token"
	"olympiadnext/internal/domain/user"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid email or password")
	ErrGoogleOnlyAccount  = errors.New("auth: account uses Google sign-in, no password set")
	ErrSessionExpired     = errors.New("auth: session expired or revoked")
	ErrInvalidOTPTarget   = errors.New("auth: type must be 'email' or 'phone'")
	ErrInvalidOTP         = errors.New("auth: invalid or expired code")
	ErrPhoneNumberNotSet  = errors.New("auth: phone number not set")
)

// otpTTL is how long a generated OTP remains valid.
const otpTTL = 5 * time.Minute

// otpCodeDigits is the alphabet SendOTP draws from to build a 6-digit code.
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
	otps          otp.Repository
	smsSender     sms.Sender
	emailSender   notifyemail.Sender
	jwtManager    *jwt.Manager
	googleVerify  *google.Verifier
	log           *slog.Logger
}

func NewService(
	users user.Repository,
	refreshTokens token.Repository,
	devices device.Repository,
	otps otp.Repository,
	smsSender sms.Sender,
	emailSender notifyemail.Sender,
	jwtManager *jwt.Manager,
	googleVerify *google.Verifier,
	log *slog.Logger,
) *Service {
	return &Service{
		users:         users,
		refreshTokens: refreshTokens,
		devices:       devices,
		otps:          otps,
		smsSender:     smsSender,
		emailSender:   emailSender,
		jwtManager:    jwtManager,
		googleVerify:  googleVerify,
		log:           log,
	}
}

func (s *Service) Register(ctx context.Context, rawEmail, password, fullName, institutionName, level, medium, deviceFingerprint string) (*TokenPair, error) {
	if err := email.ValidateEmail(rawEmail); err != nil {
		return nil, err
	}
	if err := email.ValidatePasswordStrength(password); err != nil {
		return nil, err
	}

	passwordHash, err := email.HashPassword(password)
	if err != nil {
		return nil, err
	}

	u := &user.User{
		Email:           rawEmail,
		FullName:        &fullName,
		PasswordHash:    &passwordHash,
		AuthProvider:    user.ProviderLocal,
		InstitutionName: &institutionName,
		Level:           &level,
		Medium:          &medium,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}

	s.log.Info("user registered", "user_id", u.ID, "provider", "local")
	return s.issueTokenPair(ctx, u, deviceFingerprint)
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

	s.log.Info("user logged in", "user_id", u.ID, "provider", "local")
	return s.issueTokenPair(ctx, u, deviceFingerprint)
}

// GoogleLogin verifies the ID token with Google, then either logs into
// an existing Google-linked account, links Google to an existing
// email/password account, or creates a brand new account.
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
		existing.IsEmailVerified = claims.EmailVerified
		s.log.Info("google account linked to existing user", "user_id", existing.ID)
		return s.issueTokenPair(ctx, existing, deviceFingerprint)

	case errors.Is(err, user.ErrNotFound):
		newUser := &user.User{
			Email:           claims.Email,
			FullName:        &claims.Name,
			AuthProvider:    user.ProviderGoogle,
			GoogleID:        &claims.Subject,
			IsEmailVerified: claims.EmailVerified,
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

// SendOTP generates a random 6-digit code for the given target and
// persists it with a 5-minute expiration, then delivers it: a phone
// target via smsSender (BulkSMSBD, or a console log in local dev), an
// email target via emailSender (SMTP, or a console log in local dev).
func (s *Service) SendOTP(ctx context.Context, userID string, targetType otp.TargetType) error {
	if targetType != otp.TargetEmail && targetType != otp.TargetPhone {
		return ErrInvalidOTPTarget
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if targetType == otp.TargetPhone && (u.PhoneNumber == nil || *u.PhoneNumber == "") {
		return ErrPhoneNumberNotSet
	}

	code, err := generateOTPCode()
	if err != nil {
		return fmt.Errorf("auth: generate otp failed: %w", err)
	}

	if err := s.otps.Create(ctx, &otp.OTP{
		UserID:     userID,
		TargetType: targetType,
		Code:       hash.SHA256Hex(code),
		ExpiresAt:  time.Now().Add(otpTTL),
	}); err != nil {
		return fmt.Errorf("auth: persist otp failed: %w", err)
	}

	switch targetType {
	case otp.TargetPhone:
		if err := s.smsSender.SendSMS(ctx, *u.PhoneNumber, "Your OlympiadNext OTP is "+code); err != nil {
			return fmt.Errorf("auth: send otp sms failed: %w", err)
		}
	case otp.TargetEmail:
		if err := s.emailSender.SendOTP(ctx, u.Email, code); err != nil {
			return fmt.Errorf("auth: send otp email failed: %w", err)
		}
	}

	return nil
}

// VerifyOTP checks the given code against the latest unexpired OTP issued
// for the target, consumes it on success, and marks the corresponding
// verification flag on the user.
func (s *Service) VerifyOTP(ctx context.Context, userID string, targetType otp.TargetType, code string) error {
	if targetType != otp.TargetEmail && targetType != otp.TargetPhone {
		return ErrInvalidOTPTarget
	}

	stored, err := s.otps.FindLatestValid(ctx, userID, targetType)
	if err != nil {
		if errors.Is(err, otp.ErrNotFound) {
			return ErrInvalidOTP
		}
		return err
	}

	codeMatches := subtle.ConstantTimeCompare([]byte(stored.Code), []byte(hash.SHA256Hex(code))) == 1
	if !codeMatches || !time.Now().Before(stored.ExpiresAt) {
		return ErrInvalidOTP
	}

	if err := s.otps.Delete(ctx, stored.ID); err != nil {
		return fmt.Errorf("auth: delete used otp failed: %w", err)
	}

	switch targetType {
	case otp.TargetEmail:
		err = s.users.MarkEmailVerified(ctx, userID)
	case otp.TargetPhone:
		err = s.users.MarkPhoneVerified(ctx, userID)
	}
	if err != nil {
		return fmt.Errorf("auth: mark verified failed: %w", err)
	}

	s.log.Info("otp verified", "user_id", userID, "target_type", targetType)
	return nil
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
