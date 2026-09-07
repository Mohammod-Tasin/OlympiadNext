package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"olympiadnext/internal/auth/email"
	"olympiadnext/internal/auth/jwt"
	"olympiadnext/internal/domain/token"
	"olympiadnext/internal/domain/user"
)

// The fakes below embed the domain interface, so any method a path does
// not use stays nil and panics loudly if it is ever called — e.g.
// RevokeAllForUser, the token-theft response, must never run for a benign
// rotation race.

type fakeUserRepo struct {
	user.Repository
	findByID    func(ctx context.Context, id string) (*user.User, error)
	findByEmail func(ctx context.Context, email string) (*user.User, error)
}

func (f fakeUserRepo) FindByID(ctx context.Context, id string) (*user.User, error) {
	return f.findByID(ctx, id)
}

func (f fakeUserRepo) FindByEmail(ctx context.Context, e string) (*user.User, error) {
	return f.findByEmail(ctx, e)
}

type fakeTokenRepo struct {
	token.Repository
	findByTokenHash func(ctx context.Context, h string) (*token.RefreshToken, error)
	revoke          func(ctx context.Context, id string) (bool, error)
	create          func(ctx context.Context, t *token.RefreshToken) error
}

func (f fakeTokenRepo) FindByTokenHash(ctx context.Context, h string) (*token.RefreshToken, error) {
	return f.findByTokenHash(ctx, h)
}
func (f fakeTokenRepo) Revoke(ctx context.Context, id string) (bool, error) { return f.revoke(ctx, id) }
func (f fakeTokenRepo) Create(ctx context.Context, t *token.RefreshToken) error {
	return f.create(ctx, t)
}

// newRefreshTestService wires a Service that can only exercise the refresh
// path: the user lookup always resolves the caller, and the email, device
// and Google dependencies are left nil.
func newRefreshTestService(t *testing.T, tokens token.Repository) (*Service, *jwt.Manager) {
	t.Helper()
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, 720*time.Hour)
	users := fakeUserRepo{findByID: func(_ context.Context, id string) (*user.User, error) {
		return &user.User{ID: id, Email: "user@example.com"}, nil
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(users, tokens, nil, nil, mgr, nil, log), mgr
}

func activeStoredToken(userID string) *token.RefreshToken {
	return &token.RefreshToken{ID: "stored-1", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}
}

// TestRefresh_RotationTokenCollision_Returns401 reproduces the production
// bug: a refresh whose freshly minted token is byte-identical to a row
// already stored (same user, same second, deterministic JWT) — the insert
// trips the unique index on token_hash. That must surface as a clean 401,
// not a 500. It also must not be treated as token theft: the fake token
// repo leaves RevokeAllForUser unimplemented, so reaching that path panics
// the test.
func TestRefresh_RotationTokenCollision_Returns401(t *testing.T) {
	tokens := fakeTokenRepo{
		findByTokenHash: func(_ context.Context, _ string) (*token.RefreshToken, error) {
			return activeStoredToken("user-1"), nil
		},
		revoke: func(_ context.Context, _ string) (bool, error) { return true, nil },
		create: func(_ context.Context, _ *token.RefreshToken) error {
			return token.ErrDuplicateTokenHash
		},
	}
	svc, mgr := newRefreshTestService(t, tokens)
	raw, _, err := mgr.GenerateRefreshToken("user-1")
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}

	pair, err := svc.Refresh(context.Background(), raw)
	if pair != nil {
		t.Fatalf("expected no token pair on a lost rotation race, got %+v", pair)
	}
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired (maps to HTTP 401), got %v", err)
	}
	if errors.Is(err, token.ErrDuplicateTokenHash) {
		t.Fatalf("raw persistence error leaked to the caller (would render HTTP 500): %v", err)
	}
}

func TestRefresh_HappyPath_Unchanged(t *testing.T) {
	created := 0
	tokens := fakeTokenRepo{
		findByTokenHash: func(_ context.Context, _ string) (*token.RefreshToken, error) {
			return activeStoredToken("user-1"), nil
		},
		revoke: func(_ context.Context, _ string) (bool, error) { return true, nil },
		create: func(_ context.Context, tok *token.RefreshToken) error {
			created++
			tok.ID = "new-1"
			return nil
		},
	}
	svc, mgr := newRefreshTestService(t, tokens)
	raw, _, _ := mgr.GenerateRefreshToken("user-1")

	pair, err := svc.Refresh(context.Background(), raw)
	if err != nil {
		t.Fatalf("happy-path refresh failed: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected a fully populated token pair, got %+v", pair)
	}
	if created != 1 {
		t.Fatalf("expected exactly one refresh-token insert, got %d", created)
	}
}

func TestRefresh_UnrelatedPersistenceError_StillPropagates(t *testing.T) {
	sentinel := errors.New("connection reset by peer")
	tokens := fakeTokenRepo{
		findByTokenHash: func(_ context.Context, _ string) (*token.RefreshToken, error) {
			return activeStoredToken("user-1"), nil
		},
		revoke: func(_ context.Context, _ string) (bool, error) { return true, nil },
		create: func(_ context.Context, _ *token.RefreshToken) error { return sentinel },
	}
	svc, mgr := newRefreshTestService(t, tokens)
	raw, _, _ := mgr.GenerateRefreshToken("user-1")

	_, err := svc.Refresh(context.Background(), raw)
	if !errors.Is(err, sentinel) {
		t.Fatalf("a non-collision persistence error must still propagate (HTTP 500), got %v", err)
	}
	if errors.Is(err, ErrSessionExpired) {
		t.Fatalf("an unrelated DB error was wrongly masked as a 401")
	}
}

// TestLogin_DuplicateTokenHash_IsIdempotentSuccess covers the same
// collision on the login path: two near-simultaneous logins for one
// account in the same second regenerate the identical deterministic
// token, and the second Create loses the race. The user authenticated
// correctly and the identical token is already persisted, so login must
// return that pair — not a 500.
func TestLogin_DuplicateTokenHash_IsIdempotentSuccess(t *testing.T) {
	pwHash, err := email.HashPassword("Sup3rSecret!pass")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	verified := &user.User{
		ID:            "user-1",
		Email:         "u@example.com",
		PasswordHash:  &pwHash,
		AuthProvider:  user.ProviderLocal,
		EmailVerified: true,
	}

	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, 720*time.Hour)
	users := fakeUserRepo{findByEmail: func(_ context.Context, _ string) (*user.User, error) {
		return verified, nil
	}}
	tokens := fakeTokenRepo{
		create: func(_ context.Context, _ *token.RefreshToken) error {
			return token.ErrDuplicateTokenHash // the racing login already stored it
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(users, tokens, nil, nil, mgr, nil, log)

	// Empty device fingerprint keeps the device-repo path out of this test.
	pair, err := svc.Login(context.Background(), "u@example.com", "Sup3rSecret!pass", "")
	if err != nil {
		t.Fatalf("a same-second duplicate-token race on login must be idempotent success, got: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected the (identical, already-persisted) token pair, got %+v", pair)
	}
}

// adminLoginFixture builds a Service whose only wired dependency is a user
// repo returning the given account, plus a token repo that records how
// many refresh rows were inserted. It lets the admin-login tests assert
// both the returned error and whether a session was actually minted.
func adminLoginFixture(t *testing.T, u *user.User) (*Service, *int32) {
	t.Helper()
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, 720*time.Hour)
	users := fakeUserRepo{findByEmail: func(_ context.Context, _ string) (*user.User, error) {
		return u, nil
	}}
	var inserts int32
	tokens := fakeTokenRepo{create: func(_ context.Context, tok *token.RefreshToken) error {
		atomic.AddInt32(&inserts, 1)
		tok.ID = "new-1"
		return nil
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(users, tokens, nil, nil, mgr, nil, log), &inserts
}

func verifiedUser(t *testing.T, role user.Role) *user.User {
	t.Helper()
	pwHash, err := email.HashPassword("Sup3rSecret!pass")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return &user.User{
		ID:            "user-1",
		Email:         "u@example.com",
		PasswordHash:  &pwHash,
		AuthProvider:  user.ProviderLocal,
		EmailVerified: true,
		Role:          role,
	}
}

// TestAdminLogin_NonAdmin_RejectedBeforeTokenIssued is the security half
// of the admin-cookie change: a correct student credential presented to
// the admin login path must come back as ErrAdminAccessRequired (HTTP
// 403) and must not mint or persist any refresh token.
func TestAdminLogin_NonAdmin_RejectedBeforeTokenIssued(t *testing.T) {
	svc, inserts := adminLoginFixture(t, verifiedUser(t, user.RoleStudent))

	pair, err := svc.AdminLogin(context.Background(), "u@example.com", "Sup3rSecret!pass", "")
	if !errors.Is(err, ErrAdminAccessRequired) {
		t.Fatalf("want ErrAdminAccessRequired (HTTP 403), got %v", err)
	}
	if pair != nil {
		t.Fatalf("a non-admin must not receive a token pair, got %+v", pair)
	}
	if n := atomic.LoadInt32(inserts); n != 0 {
		t.Fatalf("a non-admin must not have a refresh token persisted, got %d inserts", n)
	}
}

// TestAdminLogin_Admin_IssuesSession confirms the admin path is otherwise
// identical to Login for a legitimate admin account.
func TestAdminLogin_Admin_IssuesSession(t *testing.T) {
	svc, inserts := adminLoginFixture(t, verifiedUser(t, user.RoleAdmin))

	pair, err := svc.AdminLogin(context.Background(), "u@example.com", "Sup3rSecret!pass", "")
	if err != nil {
		t.Fatalf("admin login failed: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected a fully populated token pair, got %+v", pair)
	}
	if n := atomic.LoadInt32(inserts); n != 1 {
		t.Fatalf("expected exactly one refresh-token insert, got %d", n)
	}
}

// TestAdminLogin_WrongPassword_LooksLikeAnyBadCredential makes sure the
// role gate does not run before the password check — a bad password on
// the admin path is still ErrInvalidCredentials, not a role error.
func TestAdminLogin_WrongPassword_LooksLikeAnyBadCredential(t *testing.T) {
	svc, _ := adminLoginFixture(t, verifiedUser(t, user.RoleStudent))

	_, err := svc.AdminLogin(context.Background(), "u@example.com", "wrong-password", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

// TestLogin_NonAdmin_Unchanged pins that the ordinary student login path
// is unaffected by the admin gate.
func TestLogin_NonAdmin_Unchanged(t *testing.T) {
	svc, inserts := adminLoginFixture(t, verifiedUser(t, user.RoleStudent))

	pair, err := svc.Login(context.Background(), "u@example.com", "Sup3rSecret!pass", "")
	if err != nil {
		t.Fatalf("student login failed: %v", err)
	}
	if pair == nil || pair.AccessToken == "" {
		t.Fatalf("expected a token pair, got %+v", pair)
	}
	if n := atomic.LoadInt32(inserts); n != 1 {
		t.Fatalf("expected exactly one refresh-token insert, got %d", n)
	}
}

// TestRefresh_ConcurrentRotation_NeverErrors500 models several browser
// tabs refreshing the same cookie at once. The DB's unique index lets at
// most one rotation persist; every other concurrent caller must come back
// with a clean 401 and never an error that would render as a 500.
func TestRefresh_ConcurrentRotation_NeverErrors500(t *testing.T) {
	const n = 8
	var inserts int32
	tokens := fakeTokenRepo{
		findByTokenHash: func(_ context.Context, _ string) (*token.RefreshToken, error) {
			return activeStoredToken("user-1"), nil
		},
		revoke: func(_ context.Context, _ string) (bool, error) { return true, nil },
		create: func(_ context.Context, tok *token.RefreshToken) error {
			if atomic.AddInt32(&inserts, 1) == 1 {
				tok.ID = "new-1"
				return nil // the first rotation wins the unique index
			}
			return token.ErrDuplicateTokenHash // the rest collide
		},
	}
	svc, mgr := newRefreshTestService(t, tokens)
	raw, _, _ := mgr.GenerateRefreshToken("user-1")

	type outcome struct {
		pair *TokenPair
		err  error
	}
	results := make(chan outcome, n)
	for i := 0; i < n; i++ {
		go func() {
			p, err := svc.Refresh(context.Background(), raw)
			results <- outcome{p, err}
		}()
	}

	ok, unauthorized := 0, 0
	for i := 0; i < n; i++ {
		r := <-results
		switch {
		case r.err == nil && r.pair != nil:
			ok++
		case errors.Is(r.err, ErrSessionExpired):
			unauthorized++
		default:
			t.Fatalf("refresh returned a non-401 error under contention (would render HTTP 500): %v", r.err)
		}
	}
	if ok != 1 {
		t.Fatalf("expected exactly one winning rotation, got %d", ok)
	}
	if unauthorized != n-1 {
		t.Fatalf("expected %d clean 401s, got %d", n-1, unauthorized)
	}
}
