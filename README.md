# OlympiadNext — Auth Service

A Go backend implementing authentication (email/password with emailed OTP verification + Google Sign-In, JWT access/refresh tokens, HttpOnly cookie sessions). This is the foundation of the OlympiadNext platform — contest/olympiad domain features (problems, submissions, scoreboards, roles) are not implemented yet; today the API surface is limited to user identity and session management.

## Architecture

This repository is **backend-only**. The frontend (Next.js/TypeScript) is developed and deployed as a separate project and talks to this API cross-origin, sending credentials (cookies) with each request. Because of that split, this service is built around cross-domain deployment concerns:

- CORS is restricted to an explicit allow-list (`internal/http/middleware/cors.go`), configured via `ALLOWED_ORIGINS`.
- Refresh tokens are delivered as HttpOnly cookies with configurable `Domain`/`SameSite`/`Secure` attributes (`internal/config/config.go`) so the API can be hosted on a different domain than the frontend (e.g. a Vercel-hosted frontend + a Render/Fly-hosted API).

## Tech stack

- **Language:** Go 1.26
- **Router:** [chi](https://github.com/go-chi/chi) (`github.com/go-chi/chi/v5`)
- **Database:** PostgreSQL via `database/sql` + `github.com/lib/pq` (no ORM)
- **Auth:** `github.com/golang-jwt/jwt/v5` (JWT access/refresh tokens), `golang.org/x/crypto` (bcrypt password hashing), `google.golang.org/api` (Google ID token verification)
- **Email:** `net/smtp` over implicit TLS (port 465) for delivering verification codes
- **Rate limiting:** `golang.org/x/time/rate`
- **Local env loading:** `github.com/joho/godotenv`
- **Migrations:** a small dependency-free runner that embeds `*.sql` files with `//go:embed` and tracks applied versions in a `schema_migrations` table — no external migration tool required

## Project structure

```
main.go                          # entrypoint: config, DB connect + migrate, DI wiring, graceful shutdown
internal/
  auth/
    service.go                   # register/login/refresh/logout orchestration
    email/                       # password strength + email validation
    google/                      # Google ID token verifier
    hash/                        # bcrypt password hashing
    jwt/                         # access/refresh token issuing & parsing
  config/                        # env-based configuration, fails fast on missing secrets
  domain/
    user/                        # User entity, repository interface, domain errors
    token/                       # RefreshToken entity
  http/
    handler/                     # HTTP handlers (register, verify-email-otp, resend-email-otp, login, google, refresh, logout, me)
    middleware/                  # auth (JWT), CORS, logging, per-IP rate limiting
    dto/                         # request/response payloads
    response/                    # standard JSON response envelope
  logger/                        # slog setup
  platform/db/                   # DB connection + migration runner
    migrations/                  # embedded .sql migration files
  repository/postgres/           # Postgres implementations of the domain repositories
  server/router.go               # route table
```

## Getting started

### Prerequisites

- Go 1.26+
- A running PostgreSQL instance

### Setup

```bash
git clone https://github.com/Mohammod-Tasin/OlympiadNext.git
cd OlympiadNext
go mod download
```

Create a `.env` file in the repo root (not committed) with at least the required variables:

```dotenv
DATABASE_URL=postgres://user:password@localhost:5432/olympiadnext?sslmode=disable
JWT_ACCESS_SECRET=change-me
JWT_REFRESH_SECRET=change-me-too
GOOGLE_CLIENT_ID=your-google-oauth-client-id
```

### Environment variables

| Variable | Required | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | yes | — | Postgres connection string |
| `JWT_ACCESS_SECRET` | yes | — | Signing secret for access tokens |
| `JWT_REFRESH_SECRET` | yes | — | Signing secret for refresh tokens |
| `GOOGLE_CLIENT_ID` | yes | — | OAuth client ID used to verify Google ID tokens |
| `SMTP_HOST` | no | `smtp.gmail.com` | SMTP server for verification emails |
| `SMTP_PORT` | no | `465` | Implicit-TLS port (Render blocks 587/STARTTLS) |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | prod | *(empty)* | When unset, OTPs are logged to the console instead of emailed |
| `APP_ENV` | no | `development` | |
| `PORT` | no | `8080` | |
| `COOKIE_DOMAIN` | no | *(empty)* | Set for cross-subdomain cookie sharing |
| `COOKIE_SECURE` | no | `true` | Set `false` only for local HTTP dev |
| `COOKIE_SAME_SITE` | no | `lax` | Use `none` for cross-domain deployments (requires `COOKIE_SECURE=true`) |
| `ALLOWED_ORIGINS` | no | `http://localhost:3000` | Comma-separated list of allowed CORS origins |
| `ACCESS_TOKEN_TTL` | no | `15m` | Go duration string |
| `REFRESH_TOKEN_TTL` | no | `720h` (30 days) | Go duration string |

### Run

```bash
go run .
```

Database migrations in `internal/platform/db/migrations` are applied automatically on startup — no separate migrate step is needed.

### Build

```bash
go build -o olympiadnext .
```

## API reference

All auth routes are rate-limited per IP (30 requests/minute, burst 10).

| Method | Path | Auth required | Description |
|---|---|---|---|
| GET | `/healthz` | no | Liveness check |
| POST | `/api/auth/register` | no | Register with email + password only; emails a 6-digit code and returns no session |
| POST | `/api/auth/verify-email-otp` | no | Body `{ email, otp }`; on success marks the email verified |
| POST | `/api/auth/resend-email-otp` | no | Body `{ email }`; re-sends a code for an unverified account (uniform response) |
| POST | `/api/auth/login` | no | Log in with email + password; requires a verified email (`403` otherwise) |
| POST | `/api/auth/google` | no | Sign in / sign up with a Google ID token (`{ id_token }`) |
| POST | `/api/auth/refresh` | refresh cookie | Rotates the refresh token and issues a new access token |
| POST | `/api/auth/logout` | refresh cookie | Revokes the refresh token and clears the cookie |
| GET | `/api/auth/me` | access token | Returns the authenticated user's profile + verification status |
| POST | `/api/user/upload-file` | access token | `multipart/form-data` with a `file` field (PDF or image); returns `{ url }` |
| PUT | `/api/user/profile` | access token | Onboarding submission: academic fields + `verification_doc` (req.) + `profile_picture` (opt.); sets `verification_status` = `pending` |
| GET | `/api/client/events` | no | The single active event, or 404 |
| POST/PUT | `/api/admin/events…` | access token + admin | Create/update events and upload event images |
| GET | `/api/admin/users` | access token + admin | List users; `?status=pending\|verified\|rejected\|unverified` filters |
| PUT | `/api/admin/users/{id}/verify` | access token + admin | Body `{ "status": "verified" \| "rejected" }` |
| GET | `/uploads/*` | no | Event images |
| GET | `/uploads/users/{userID}/{name}` | access token | KYC files — owner or admin only |

The refresh token is set as an HttpOnly cookie; the access token is returned in the JSON response body and expected in the `Authorization: Bearer <token>` header on protected routes.

### Email verification flow

1. `POST /api/auth/register` creates the account with `email_verified = false`, stores a 6-digit OTP (5-minute expiry), and emails it.
2. `POST /api/auth/verify-email-otp` with `{ email, otp }` sets `email_verified = true` and clears the code. `POST /api/auth/resend-email-otp` issues a fresh one if needed.
3. `POST /api/auth/login` succeeds only once the email is verified.

Google accounts are created already-verified with no password.

### Student verification (KYC) flow

1. After logging in, the student `POST`s their proof document (PDF or image) and, optionally, a profile picture to `/api/user/upload-file`. Each call returns a `{ url }` under `/uploads/users/<userID>/`.
2. `PUT /api/user/profile` submits the academic fields plus `verification_doc` (and optional `profile_picture`). This sets `verification_status` to `pending`.
3. An admin lists the queue with `GET /api/admin/users?status=pending`, fetches each `verification_doc` (admins may read any `/uploads/users/*` file with their token), and decides with `PUT /api/admin/users/{id}/verify`.
4. A `rejected` student can re-run steps 1–2, returning to `pending`.

`verification_status` is one of `unverified` (default), `pending`, `verified`, `rejected`.

## Security features

- Passwords hashed with bcrypt (`internal/auth/hash`), plus email/password validation before hashing
- JWT access + refresh token pairs, with refresh tokens rotated on use and stored server-side as SHA-256 hashes rather than raw values
- Per-IP rate limiting on all `/api/auth/*` routes
- CORS restricted to an explicit origin allow-list
- Cookie flags (`Domain`, `Secure`, `SameSite`) configurable for same-domain or cross-domain deployments

## Testing

No automated tests exist yet. `go test ./...` will run cleanly with no test files.

## Status / roadmap

This repo currently implements only the auth foundation of OlympiadNext. Olympiad/contest-specific domain concepts (contests, problems, submissions, scoring, roles) are not yet built.
