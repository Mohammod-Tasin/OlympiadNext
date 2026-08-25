# OlympiadNext — Auth Service

A Go backend implementing authentication (email/password + Google Sign-In, JWT access/refresh tokens, HttpOnly cookie sessions). This is the foundation of the OlympiadNext platform — contest/olympiad domain features (problems, submissions, scoreboards, roles) are not implemented yet; today the API surface is limited to user identity and session management.

## Architecture

This repository is **backend-only**. The frontend (Next.js/TypeScript) is developed and deployed as a separate project and talks to this API cross-origin, sending credentials (cookies) with each request. Because of that split, this service is built around cross-domain deployment concerns:

- CORS is restricted to an explicit allow-list (`internal/http/middleware/cors.go`), configured via `ALLOWED_ORIGINS`.
- Refresh tokens are delivered as HttpOnly cookies with configurable `Domain`/`SameSite`/`Secure` attributes (`internal/config/config.go`) so the API can be hosted on a different domain than the frontend (e.g. a Vercel-hosted frontend + a Render/Fly-hosted API).

## Tech stack

- **Language:** Go 1.26
- **Router:** [chi](https://github.com/go-chi/chi) (`github.com/go-chi/chi/v5`)
- **Database:** PostgreSQL via `database/sql` + `github.com/lib/pq` (no ORM)
- **Auth:** `github.com/golang-jwt/jwt/v5` (JWT access/refresh tokens), `golang.org/x/crypto` (bcrypt password hashing), `google.golang.org/api` (Google ID token verification)
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
    handler/                     # HTTP handlers (register, login, google, refresh, logout, me)
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
| POST | `/api/auth/register` | no | Register with email + password |
| POST | `/api/auth/login` | no | Log in with email + password |
| POST | `/api/auth/google` | no | Sign in / sign up with a Google ID token |
| POST | `/api/auth/refresh` | refresh cookie | Rotates the refresh token and issues a new access token |
| POST | `/api/auth/logout` | refresh cookie | Revokes the refresh token and clears the cookie |
| GET | `/api/auth/me` | access token | Returns the authenticated user's id and email |

The refresh token is set as an HttpOnly cookie; the access token is returned in the JSON response body and expected in the `Authorization: Bearer <token>` header on protected routes.

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
