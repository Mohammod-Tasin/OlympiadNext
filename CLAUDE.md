# CLAUDE.md

## What this is

`olympiadnext` — a Go 1.26 **backend-only** auth service (module name `olympiadnext`).
The Next.js frontend lives in a separate repo and calls this API cross-origin with
credentials, which is why cookie flags and CORS are so configurable.

Authentication is email/password (with an emailed OTP) plus Google OAuth. There
is no phone/SMS auth — it was removed.

Built so far: the auth/identity foundation, `student`/`admin` roles, admin-curated
**events** (client-facing content blocks + image upload), and a manual **student
verification (KYC)** flow (upload a proof document, admin approves/rejects).
Contests, problems, submissions, and scoring are **not built yet**.

## Commands

```bash
go run .                 # runs on :8080; migrations apply automatically at startup
go build -o olympiadnext .
go vet ./...
go test ./...            # passes trivially — there are no test files yet
```

Requires a live Postgres. Config comes from `.env` (gitignored) via godotenv; see
`internal/config/config.go` for the full list. Required or the server exits:
`DATABASE_URL`, `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`, `GOOGLE_CLIENT_ID`.
SMTP (`SMTP_*`) creds are optional — when unset, the sender logs the OTP to the
console so local dev works without live delivery.

## Layout

Layered, hand-wired dependency injection in `main.go` (no DI framework):

```
main.go                      config → db connect+migrate → repos → services → handlers → router
internal/config/             env loading, fails fast on missing secrets
internal/domain/             entities + repository INTERFACES + sentinel errors
  user/ token/ device/ email/ event/
internal/auth/               service.go = all orchestration; sub-pkgs jwt/ hash/ google/ email/
internal/app/events/         event service (admin content orchestration)
internal/repository/postgres/ implementations of the domain interfaces
internal/http/               handler/ (auth, user, admin, event) middleware/ dto/ response/
internal/platform/           db/ (+ embedded migrations/), email/ (SMTP), storage/ (local uploads)
internal/server/router.go    the whole route table
```

Dependency direction: `http` → `auth` → `domain` ← `repository`. Domain packages
define interfaces; `postgres` and `platform` implement them. Nothing in `domain`
imports anything outward.

## Routes (`internal/server/router.go`)

All of `/api/auth/*` is rate-limited per IP (30 req/min, burst 10). State-changing
routes additionally require a trusted `Origin` (`RequireTrustedOrigin`).

| Route | Auth |
|---|---|
| `GET /`, `HEAD /`, `GET /healthz` | none (Render health checks) |
| `POST /api/auth/register` (email + password only), `/login`, `/google` | none |
| `POST /api/auth/verify-email-otp`, `/resend-email-otp` | none |
| `POST /api/auth/refresh`, `/logout` | refresh cookie |
| `GET /api/auth/me` | access token |
| `POST /api/user/upload-file` (multipart `file`; PDF or image) | access token |
| `PUT /api/user/profile` (onboarding + profile edits: academic fields, optional `verification_doc`) | access token |
| `GET /api/client/events` | none |
| `POST /api/admin/events`, `/events/upload`, `PUT /api/admin/events/{id}` | access token + admin |
| `GET /api/admin/users?status=` , `PUT /api/admin/users/{id}/verify` | access token + admin |
| `GET /uploads/*` (event images) | none |
| `GET /uploads/users/{userID}/{name}` (KYC files) | access token; owner or admin only |

## Conventions that matter

- **Tokens.** Access JWT goes in the JSON body, sent back as `Authorization: Bearer`.
  Refresh token is an HttpOnly cookie, rotated on every use, and stored server-side
  only as a SHA-256 hex digest (`hash.SHA256Hex`) — never in plaintext.
- **Single-device sessions.** Clients send `X-Device-Fingerprint`; `issueTokenPair`
  writes it to `users.active_device_fingerprint`, and `RequireAccessToken` rejects
  requests whose fingerprint doesn't match the active one. Logging in elsewhere
  kicks the previous device.
- **Email verification.** `/register` creates the account with
  `users.email_verified = false`, then mails a 6-digit OTP stored on the user row
  (`email_otp` / `email_otp_expiry`, 5-minute TTL). It issues **no** tokens.
  `/verify-email-otp` flips the flag and nullifies the code in one UPDATE, so a
  code can't be replayed. `/login` refuses an unverified account with 403.
  `/send-email-otp` re-issues a code and always answers with the same generic
  message so it can't enumerate accounts. Google sign-in skips all of this —
  Google has already confirmed the address. OTP delivery is best-effort: the
  SMTP sender wraps a failed send in `email.ErrDeliveryFailed`, and
  `issueEmailOTP` logs the code as a `WARN` and returns success rather than
  failing the request — Render blocks outbound SMTP (465/587), so a live send
  times out there every time.
- **Student verification (KYC).** `users.verification_status` moves
  `unverified → pending → verified | rejected` (rejected users may resubmit).
  `POST /api/user/upload-file` stores a PDF/image under `uploads/users/<userID>/`
  and returns its URL; `PUT /api/user/profile` serves both onboarding and later
  profile edits — it always saves the academic fields and an optional
  `profile_picture`. `verification_doc` is optional: a new value replaces the
  stored document and flips the status to `pending`; omitting it keeps the
  document and status already on file, so a verified user can edit their name
  without re-entering review. Admins review via `GET /api/admin/users?status=pending` and
  decide with `PUT /api/admin/users/{id}/verify` (`{"status":"verified"|"rejected"}`).
  KYC files are identity documents: `/uploads/users/*` is served only to the owning
  user or an admin (Bearer token), while event images under `/uploads/*` stay public.
- **Profile completeness.** `middleware.RequireCompleteProfile` gates future
  non-auth routes on verified email, `verification_status = verified`, and full
  name/institution/level/medium. Deliberately not applied to `/api/auth/*` or
  `/api/user/*` so users can finish onboarding.
- **Migrations.** Add a numbered pair `NNNN_name.up.sql` / `.down.sql` under
  `internal/platform/db/migrations/`. They're `//go:embed`-ed and applied in
  filename order by a homegrown runner tracking `schema_migrations`; there is no
  external migration CLI, and down-migrations are never run automatically.
- **Errors.** Domain packages export sentinels (`user.ErrNotFound`,
  `user.ErrEmailTaken`, `otp.ErrNotFound`, …); handlers map them to status codes in
  `handleAuthError`. Wrap with `fmt.Errorf("pkg: what failed: %w", err)`.
- **Responses.** Always via `internal/http/response` — `response.JSON` /
  `response.Error` (which emits `{"error": "..."}`). Don't hand-roll `w.Write`.
- **Logging.** `log/slog` with key-value pairs, injected as `*slog.Logger`; no
  package-level logger, no `fmt.Println`.
- **SQL.** `database/sql` + `lib/pq`, no ORM, `$1` placeholders.
- Comments in this codebase explain *why* a non-obvious choice was made (e.g. why
  SMTP port 465, why `SameSite=none`). Match that — skip comments that restate code.

## Notes

- Working branch is `windows`; `main` is the default branch.
- Deployed to Render — hence port 465 for SMTP (587 is blocked outbound) and the
  explicit `/` handler.
- `README.md` is user-facing and stale: it predates the device and
  academic-profile work and still documents phone/SMS auth. Trust the code
  over it.
