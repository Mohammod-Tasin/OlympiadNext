# CLAUDE.md

## What this is

`olympiadnext` — a Go 1.26 **backend-only** auth service (module name `olympiadnext`).
The Next.js frontend lives in a separate repo and calls this API cross-origin with
credentials, which is why cookie flags and CORS are so configurable.

Authentication is email/password (with an emailed OTP) plus Google OAuth. There
is no phone/SMS auth — it was removed.

Built so far: the auth/identity foundation, `student`/`admin` roles, admin-curated
**events** (client-facing content blocks + image upload), an admin-curated
bilingual **notice board**, and a manual **student verification (KYC)** flow
(upload a proof document, admin approves/rejects).
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
console so local dev works without live delivery. `BULKSMSBD_API_KEY` /
`BULKSMSBD_SENDER_ID` (SMS) are optional too and never block startup — an
unset key fails only at send time with `sms.ErrNotConfigured`.

## Layout

Layered, hand-wired dependency injection in `main.go` (no DI framework):

```
main.go                      config → db connect+migrate → repos → services → handlers → router
internal/config/             env loading, fails fast on missing secrets
internal/domain/             entities + repository INTERFACES + sentinel errors
  user/ token/ device/ email/ event/ notice/ registration/ sms/
internal/auth/               service.go = all orchestration; sub-pkgs jwt/ hash/ google/ email/
internal/app/events/         event service (admin content orchestration)
internal/app/notices/        notice-board service (admin CRUD + public list)
internal/app/registrations/  exam-registration payment + admit-card orchestration
internal/app/notify/         notification-channel preference, phone OTP, admit-card dispatch
internal/repository/postgres/ implementations of the domain interfaces
internal/http/               handler/ (auth, user, admin, event, notice, registration, notification) middleware/ dto/ response/
internal/platform/           db/ (+ embedded migrations/), email/ (SMTP), sms/ (BulkSMSBD), storage/ (local uploads)
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
| `POST /api/auth/refresh`, `/logout` | refresh cookie (`refresh_token`, path `/api/auth`) |
| `GET /api/auth/me` | access token |
| `POST /api/auth/admin/login` | none; rejects non-admin with 403 before issuing tokens |
| `POST /api/auth/admin/refresh`, `/logout` | admin refresh cookie (`admin_refresh_token`, path `/api/auth/admin`) |
| `POST /api/user/upload-file` (multipart `file`; PDF or image) | access token |
| `PUT /api/user/profile` (onboarding + profile edits: academic fields, optional `verification_doc`) | access token |
| `POST /api/user/registrations` (exam registration: `event_id`, `payment_method`, `sender_number`, `transaction_id`) | access token |
| `GET /api/user/registrations` (the caller's own registrations + status, incl. `admit_card_url`) | access token |
| `PUT /api/user/notification-preference` (`{method:"email"\|"phone", phone}`; phone sends an SMS OTP, channel stays `email` until verified) | access token |
| `POST /api/user/notification-phone/verify-otp` (`{otp}`), `/notification-phone/resend-otp` (no body) | access token |
| `GET /api/client/events` (includes per-event `bkash_number`, `nagad_number`, `registration_fee`; `is_registered` reflects the caller's own exam registration when a valid access token is sent) | none (optional access token) |
| `GET /api/client/notices` (active notices only, `display_order` ASC; each row carries `text_en` + `text_bn`) | none |
| `POST /api/admin/events`, `/events/upload`, `PUT /api/admin/events/{id}` | access token + admin |
| `GET /api/admin/notices` (all notices, active or not), `POST /api/admin/notices`, `PUT /api/admin/notices/{id}` (`text_en`, `text_bn`, `display_order`, `is_active`), `DELETE /api/admin/notices/{id}` | access token + admin |
| `GET /api/admin/users?status=` , `PUT /api/admin/users/{id}/verify` | access token + admin |
| `GET /api/admin/registrations?status=&event_id=` , `PUT /api/admin/registrations/{id}/review` (`{"status":"approved"\|"rejected"}`), `PUT /api/admin/registrations/{id}/unreject` (no body) | access token + admin |
| `POST /api/admin/registrations/{id}/admit-card` (multipart `file`, PDF only; 409 unless the registration is `approved`) | access token + admin |
| `GET /uploads/*` (event images) | none |
| `GET /uploads/users/{userID}/{name}` (KYC files), `GET /uploads/admit-cards/{userID}/{name}` (admit cards) | access token; owner or admin only |

## Conventions that matter

- **Tokens.** Access JWT goes in the JSON body, sent back as `Authorization: Bearer`.
  Refresh token is an HttpOnly cookie, rotated on every use, and stored server-side
  only as a SHA-256 hex digest (`hash.SHA256Hex`) — never in plaintext.
- **Two refresh cookies.** The student site and the admin console are separate
  Vercel origins hitting this one backend, so the browser keys the refresh cookie
  only by (name, path) on the backend domain. `/api/auth/*` uses `refresh_token` at
  path `/api/auth`; `/api/auth/admin/*` uses `admin_refresh_token` at path
  `/api/auth/admin`. Both are served by the same `AuthHandler` type and the same
  `auth.Service` — `handler.NewAdminAuthHandler` just swaps the `cookieConfig`
  name/path and points `Login` at `authService.AdminLogin` (credential check +
  `role == admin`, else `ErrAdminAccessRequired` → 403 with no token issued).
  Keeps the two sessions independent in one browser.
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
- **Exam registration (manual payment).** A student pays an event's
  `registration_fee` to its `bkash_number` / `nagad_number` (per-event
  columns, not a global setting) and submits the wallet TrxID via
  `POST /api/user/registrations`. `exam_registrations.status` moves
  `pending → approved | rejected`; an admin decides via
  `PUT /api/admin/registrations/{id}/review`, which stamps `reviewed_by`
  (the authenticated admin) and `reviewed_at`. A narrowly scoped admin
  correction, `PUT /api/admin/registrations/{id}/unreject`, permits only
  `rejected → pending` and clears those review fields; it logs the admin and
  registration IDs. Two UNIQUE constraints make fraud/dupes hard — there is
  still **no** second submission: `uq_exam_reg_transaction_id` (a TrxID backs one
  registration ever, by anyone) and `uq_exam_reg_user_event` (one
  registration per student per exam). The repo maps each violation to its
  own sentinel (`ErrDuplicateTransactionID` / `ErrAlreadyRegistered` →
  distinct 409s). Registration does **not** require `verification_status =
  verified` — any authenticated student may submit; the manual payment
  check is the fraud gate. The submit endpoint sits in the `/api/user`
  group so it shares that tier's per-IP rate limit. TrxID is upper-cased
  and `sender_number` normalised to `01XXXXXXXXX` before persistence.
  `GET /api/client/events` stays public but parses an optional access
  token itself (like `serveProtectedFile`); with a valid one it adds
  `is_registered` — a single `SELECT EXISTS` on `exam_registrations` for
  `user_id + event_id`, true for a row in any status — so the frontend
  can hide the payment form. A failed check logs and leaves the flag
  `false` rather than failing the page.
- **Admit cards.** Once a registration is `approved`, an admin uploads a
  PDF admit card via `POST /api/admin/registrations/{id}/admit-card`. It is
  stored under `uploads/admit-cards/<studentUserID>/`, only the path is
  persisted (`exam_registrations.admit_card_url` + `admit_card_uploaded_at`),
  and the file server gates it owner-or-admin exactly like KYC files (same
  `serveProtectedFile` helper, `/uploads/admit-cards/*` refused by the
  public server). `SetAdmitCard` re-checks `status = 'approved'` in SQL, so
  a concurrent unreject cannot slip a card onto a non-approved row
  (`registration.ErrNotApproved` → 409). After a successful upload the
  student is notified on their chosen channel (see Notifications).
- **Notifications & phone OTP.** `users.notification_method` (`email` default
  / `phone`) is the channel for transactional alerts (currently only the
  admit-card-ready message). A student opts into SMS with
  `PUT /api/user/notification-preference` (`{method:"phone", phone}`), which
  stores the number and sends a 6-digit OTP but leaves the active channel on
  `email`; `POST /api/user/notification-phone/verify-otp` flips
  `notification_phone_verified` and `notification_method` to `phone` in one
  UPDATE that also nullifies the code. The phone-OTP flow mirrors the email
  OTP exactly (`crypto/rand`, 5-minute TTL, constant-time compare,
  best-effort delivery — a failed send logs the code as WARN and still
  succeeds). These columns are deliberately separate from the phone/SMS
  *auth* fields migration 0011 removed — this is a delivery preference, not
  a login identity. SMS goes through BulkSMSBD (`internal/platform/sms`);
  `BULKSMSBD_API_KEY` / `BULKSMSBD_SENDER_ID` are **optional** and never
  block startup — an unset key surfaces as `sms.ErrNotConfigured` only on a
  send attempt, logged at ERROR. `notify.Service.NotifyAdmitCardReady`
  routes to SMS only when the user selected `phone` *and* verified it,
  falling back to email otherwise.
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
  `ServeUserFile` validates the token and enforces owner-or-admin itself instead of
  using `RequireAccessToken` — a browser can't put `X-Device-Fingerprint` on an
  `<img>`/download request, so the single-device gate would 401 every document view.
  It answers 401 only for a missing/invalid token and 403 for a valid token that is
  neither the owner nor an admin.
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


Before editing any file, always propose a plan first and wait for explicit approval — do not write code until the plan is approved, especially for anything touching auth, JWT, OTP, or KYC logic