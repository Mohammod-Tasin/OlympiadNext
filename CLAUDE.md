# CLAUDE.md

## What this is

`olympiadnext` — a Go 1.26 **backend-only** auth service (module name `olympiadnext`).
The Next.js frontend lives in a separate repo and calls this API cross-origin with
credentials, which is why cookie flags and CORS are so configurable.

Only the auth/identity foundation exists. Contests, problems, submissions, scoring,
and roles are **not built yet** — don't assume they exist.

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
SMS (`BULKSMSBD_*`) and SMTP (`SMTP_*`) creds are optional — when unset, both
senders log the OTP to the console so local dev works without live delivery.

## Layout

Layered, hand-wired dependency injection in `main.go` (no DI framework):

```
main.go                      config → db connect+migrate → repos → services → handlers → router
internal/config/             env loading, fails fast on missing secrets
internal/domain/             entities + repository INTERFACES + sentinel errors
  user/ token/ device/ otp/ email/ sms/
internal/auth/               service.go = all orchestration; sub-pkgs jwt/ hash/ google/ email/
internal/repository/postgres/ implementations of the domain interfaces
internal/http/               handler/ middleware/ dto/ response/
internal/platform/           db/ (+ embedded migrations/), email/ (SMTP), sms/ (BulkSMSBD)
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
| `POST /api/auth/register`, `/login`, `/google` | none |
| `POST /api/auth/refresh`, `/logout` | refresh cookie |
| `GET /api/auth/me` | access token |
| `POST /api/auth/send-otp`, `/verify-otp`, `/update-phone` | access token |
| `PUT /api/auth/profile` | access token |

## Conventions that matter

- **Tokens.** Access JWT goes in the JSON body, sent back as `Authorization: Bearer`.
  Refresh token is an HttpOnly cookie, rotated on every use, and stored server-side
  only as a SHA-256 hex digest (`hash.SHA256Hex`) — never in plaintext.
- **Single-device sessions.** Clients send `X-Device-Fingerprint`; `issueTokenPair`
  writes it to `users.active_device_fingerprint`, and `RequireAccessToken` rejects
  requests whose fingerprint doesn't match the active one. Logging in elsewhere
  kicks the previous device.
- **OTPs.** 6-digit, 5-minute TTL, stored hashed, single-use (verification *deletes*
  the row). Targets are `email` or `phone` (`otp.TargetType`).
- **Profile completeness.** `middleware.RequireCompleteProfile` gates future
  non-auth routes on verified email + phone and full name/institution/level/medium.
  Deliberately not applied to `/api/auth/*` so users can finish onboarding.
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
- `README.md` is user-facing and partly stale: it predates the OTP, device, phone,
  and academic-profile work. Trust the code over it.
