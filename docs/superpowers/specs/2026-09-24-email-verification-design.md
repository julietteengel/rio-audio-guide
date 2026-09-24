# Email verification (AWS SES)

**Status:** design approved, spec pending implementation plan.

## Why

`POST /register` currently creates an active account immediately — no email sent, no confirmation,
nothing verifying the address actually belongs to the person registering. Discovered during live
end-to-end testing on 2026-09-24 while validating the app against the real deployed backend. The
founder wants this closed before broader testing continues.

**Accepted constraint:** AWS SES starts in "sandbox" mode — it can only send to recipient addresses
that have themselves been manually verified in the SES console, until AWS approves a production-access
request (not instant). This spec builds the feature to work correctly once SES is out of sandbox; until
then, only manually-verified test addresses can complete registration. That's an operational limitation
of the AWS account, not a gap in this design.

## Scope

**In scope (v1):**
- A 6-digit numeric code, emailed on registration, valid 15 minutes, required to activate the account.
- Login is blocked (not just "unverified" cosmetically — a real 403) until the code is submitted and
  matches.
- A resend-code endpoint, for an expired or lost code.
- A new AWS SES-backed adapter for sending the email, following this project's existing adapter pattern
  (see `internal/adapters/awspolly/generator.go` for the established shape: a thin adapter over an AWS
  SDK v2 client, constructed once in `main.go` and injected via a port interface).
- A new mobile screen for code entry, inserted into the registration flow before the app is usable.

**Explicitly out of scope for v1 (deferred, not decided now):**
- Clickable email links / deep-linking into the app (the mobile app has no `scheme` configured in
  `app.json` today — adding one is a separate, small piece of native config plus a rebuild; a typed
  6-digit code sidesteps it entirely for v1).
- Domain-based sender identity (a real "noreply@domain" address, DNS-verified) — v1 verifies a single
  email address as the SES sender identity, the faster of SES's two verification paths. Revisit once
  the project has a real domain in production use.
- Rate-limiting resend requests beyond what SES itself enforces — no dedicated cooldown/backoff logic
  in this app for v1.
- Any change to `POST /me` (email change) re-triggering verification — out of scope, existing behavior
  (an email change takes effect immediately) is untouched.

## Data model

`schema.sql`'s `users` table gains three columns:
```sql
email_verified               BOOLEAN     NOT NULL DEFAULT false,
verification_code            TEXT,
verification_code_expires_at TIMESTAMPTZ
```
`email_verified` defaults `false` for new rows. The two verification-code columns are nullable — a
verified account eventually has no live code (cleared on success), and existing rows in already-deployed
databases (local dev, the EC2 test deployment) start with no code and `email_verified = false`
retroactively; this is acceptable for v1 since only the founder's own already-tested accounts exist
there — she re-verifies once, same as any new account.

Kept as plain columns on `users`, not a separate table/aggregate: a verification code is a single,
replaceable, short-lived value per user (not a history worth preserving), no different in spirit from
`password_hash` already living directly on the row. This mirrors the project's existing convention that
credential-adjacent secrets are opaque strings on the entity, generated/checked in the application layer,
never given domain logic of their own.

## Domain (`internal/domain/user.go`)

- `User` gains a field `emailVerified bool`.
- `NewUser(...)` (used by registration) sets `emailVerified: false`.
- `ReconstructUser(...)` (used by the Postgres repository to rebuild from a stored row) gains a new
  `emailVerified bool` parameter, appended after `status` — every call site in
  `internal/adapters/postgres/user_repository.go` updates to pass the newly-read column.
- New accessor: `func (u *User) EmailVerified() bool`.
- New method: `func (u *User) MarkEmailVerified() error` — returns `ErrUserDeleted` if the account is
  deleted (same guard every other mutator on this entity already uses), otherwise sets the field `true`.
  Idempotent: calling it on an already-verified user is a harmless no-op, not an error — there's no
  scenario where re-submitting a still-valid code should surface as a client-visible failure.

The verification code's actual value, generation, and expiry are deliberately NOT domain concerns —
same reasoning `PasswordHash` already documents for hashing: "a problème technique, pas une règle
métier." Code generation and expiry-checking live in the application layer (below).

## Ports (`internal/ports/`)

New port, `internal/ports/email_sender.go`:
```go
package ports

import "context"

type EmailSender interface {
    SendVerificationCode(ctx context.Context, toEmail, code string) error
}
```

`UserRepository` (`internal/ports/user_repository.go`) gains two methods:
```go
SaveVerificationCode(ctx context.Context, userID, code string, expiresAt time.Time) error
FindVerificationCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
```
`FindVerificationCode` returns a not-found error (the same sentinel pattern `FindByID`/`FindByEmail`
already use — `pgx.ErrNoRows` surfaced as-is, per this project's established convention of doing
status-code translation at the HTTP layer, not inside `application`) when no code is currently stored
for that user (e.g., already verified and cleared, or never registered).

## Application (`internal/application/`)

**`RegisterUser`** (modified): after `userRepo.Save(ctx, user)` succeeds, generate a 6-digit numeric
code (`crypto/rand`, not `math/rand` — this is a credential, however short-lived), set its expiry to
`time.Now().Add(15 * time.Minute)`, persist via `SaveVerificationCode`, then call
`emailSender.SendVerificationCode(ctx, user.Email().String(), code)`. The function signature gains
`emailSender ports.EmailSender` as a new parameter. If sending fails, the registration itself still
succeeds (the account and stored code both already persisted) — return the error to the caller so the
HTTP layer can report "account created, but the email failed to send, use resend" rather than
pretending nothing happened; do not roll back the created user over a transient email-provider failure.

**`VerifyEmail`** (new): `func VerifyEmail(ctx context.Context, userRepo ports.UserRepository, email, code string) error`.
Looks up the user by email, then their stored code via `FindVerificationCode`. Compares the submitted
code against the stored one (constant-time compare, `crypto/subtle.ConstantTimeCompare` — this is a
short numeric code checked over the network, worth the trivial cost to avoid a timing side-channel).
Checks `time.Now()` against the stored expiry. On success, calls `user.MarkEmailVerified()`, saves the
user, and clears the stored code (a fresh `SaveVerificationCode` call with an empty code and a
zero/past expiry, or a dedicated clear — implementation detail for the plan). New error,
`ErrVerificationCodeInvalid`, returned for a wrong code, an expired one, or no code found at all — the
HTTP layer maps all three to the same response, deliberately not distinguishing "wrong" from "expired"
to a client (no operational reason to leak which).

**`ResendVerificationCode`** (new): `func ResendVerificationCode(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email string) error`.
Looks up the user by email, generates a fresh code (same generation logic as registration, factored into
a small shared helper rather than duplicated), overwrites the stored one, resends. Silently succeeds
(returns nil) even if the user doesn't exist or is already verified — mirrors this project's existing
anti-enumeration stance on `/login` (`ErrInvalidCredentials` never distinguishes "no such email" from
"wrong password"); a resend endpoint that reveals which emails have accounts would be a regression from
that stance.

**`LoginUser`** (modified): after the existing password check succeeds, add a check:
`if !user.EmailVerified() { return "", ErrEmailNotVerified }`. New sentinel error,
`ErrEmailNotVerified`, distinct from `ErrInvalidCredentials` — the HTTP layer maps it to a different
status/body so the mobile client can route to the verification screen instead of showing a generic
"wrong password" message. This is not an enumeration risk: the caller already proved they know the
correct password for this email, so confirming "this specific account needs verification" adds no
information an attacker didn't already have.

## Adapter (`internal/adapters/awsses/sender.go`, new package)

Follows `internal/adapters/awspolly/generator.go`'s established shape: a small struct wrapping an AWS
SDK v2 client, constructed once in `cmd/api/main.go` (and `cmd/worker/main.go` only if a later feature
needs it — registration/login both happen through the `api` process, so `worker` doesn't need this
adapter for v1) via the same shared AWS config/credentials loading already used for the S3 and Polly
clients, and injected as `ports.EmailSender` wherever `RegisterUser`/`ResendVerificationCode` are wired
up.

Uses `github.com/aws/aws-sdk-go-v2/service/sesv2` (new dependency — not yet in `go.mod`; the AWS SDK v2
root module and shared config loading are already present via the Polly/S3 adapters, so this is an
incremental addition, not a new SDK generation). `SendVerificationCode` calls SES's `SendEmail` API with
a plain-text body (no HTML templating needed for a 6-digit code) containing the code and a one-line
explanation, addressed from a single verified sender identity (see AWS setup below), configured via an
environment variable (`SES_SENDER_EMAIL`, following the existing `.env`-driven configuration pattern
already used for `ANTHROPIC_API_KEY`/AWS credentials/etc.).

## HTTP (`internal/adapters/http/`)

- `POST /register` (existing route, `registerUser` handler): unchanged request/response shape
  (`registerRequest` → `201` + `userResponse`). Internally now also triggers the email send via the
  modified `RegisterUser` application function. If the email send specifically failed (registration
  itself still succeeded), respond `201` still (the account exists and works once verified) but note
  the send failure isn't silently swallowed — server-side logged, and the client can always fall back to
  the resend endpoint regardless of why the first send didn't arrive.
- `POST /verify-email` (new): body `{"email": string, "code": string}`. Calls `application.VerifyEmail`.
  `200` + `{}` on success. `422` + `{"error": "invalid or expired code"}` on
  `ErrVerificationCodeInvalid` (matches this project's existing convention of `422` for
  domain/application-level rejections, e.g. `registerUser`'s existing error mapping).
- `POST /resend-verification-code` (new): body `{"email": string}`. Calls
  `application.ResendVerificationCode`. Always `200` + `{}` (per the anti-enumeration behavior described
  above — the endpoint never signals whether the address exists).
- `POST /login` (existing route, `login` handler): on `errors.Is(err, application.ErrEmailNotVerified)`,
  return `403` + `{"error": "email not verified"}` — a distinct status from the existing `401` for
  `ErrInvalidCredentials`, so the mobile client can branch on status code alone without parsing the
  error string.

## Mobile (`mobile/src/`)

- `data/AuthRepository.ts`: two new functions, `verifyEmail(email, code)` and
  `resendVerificationCode(email)`, following the existing `authFetch` helper's pattern (same file
  already has `register`/`login`/`logout`/`updateProfile`/`deleteAccount` built the same way).
- `auth/AuthContext.tsx`: `registerFn` currently calls `Auth.register(...)` then immediately
  `Auth.login(...)` to auto-sign-in. This changes: `registerFn` still calls `Auth.register(...)`, but
  does NOT auto-login afterward (login would now fail with `403` anyway, since the fresh account isn't
  verified yet). Instead, `AuthScreen`'s `submit()` (currently calling `register(email, password)` then
  `navigation.goBack()`) navigates to a new `VerifyEmail` screen on successful registration, carrying
  `email`/`password` as route params (password is needed to complete the login automatically once
  verification succeeds, so the user isn't asked to type it twice).
- New screen `screens/VerifyEmail.tsx`: a single 6-digit code input, a submit button calling
  `verifyEmail(email, code)`, and a "resend code" link calling `resendVerificationCode(email)`. On
  successful verification, calls `AuthContext`'s existing `login(email, password)` (completing the
  originally-intended auto-login) and navigates into the app, same as today's post-registration flow.
  On a login failure specifically because of `403`/not-verified (a genuinely new case the existing
  `AuthScreen.submit()` catch block doesn't yet distinguish), also route to `VerifyEmail` — covers a
  user who registered, closed the app before verifying, and later tries to log in normally.
- New dictionary keys (`mobile/src/i18n/dictionary.ts`, all 4 locales): a `verifyEmail` section
  (title, subtitle, code input placeholder, resend link text, an error string for an invalid/expired
  code) plus one new `auth.emailNotVerified` string for the login-screen-redirect case.

## AWS setup (manual, founder's own AWS console work — not automatable from here)

1. **Verify a sender identity** in SES (Simple Email Service console → Identities → Create identity →
   Email address) — a single address (her own), not a full domain; SES emails a confirmation link to
   that address, click it, done in a couple of minutes. This becomes `SES_SENDER_EMAIL`.
2. **Verify each recipient address used for testing** the same way, while still in sandbox mode — every
   account she registers during testing needs its email address separately verified in SES first, or
   the send will fail (not a bug, this is SES's sandbox restriction working as designed).
3. **Grant `ses:SendEmail`/`ses:SendRawEmail`** to the `rio-cicd` IAM user (the one the deployed backend
   already uses for S3) — via IAM console, attach a policy (either the AWS-managed `AmazonSESFullAccess`
   for speed, or a scoped inline policy restricted to those two actions on the verified sender identity's
   ARN, matching `rio-cicd`'s existing minimal-permissions posture more closely — implementer's call
   during the plan, both are legitimate, the scoped one is preferable if it doesn't cost meaningful time).
4. Request AWS to lift sandbox mode (SES console → Account dashboard → "Request production access") —
   not blocking for v1 testing, but the eventual unblock for real, non-preverified users. Not part of
   this implementation plan's work (nothing to code for it) — flagging here so it isn't forgotten.

## Testing

- Domain: `MarkEmailVerified()`'s guard (rejects on a deleted user, succeeds/no-ops otherwise) — unit
  test, same style as the existing `ChangeEmail`/`ChangePassword`/`Delete` tests already in
  `internal/domain/user_test.go`.
- Application: `VerifyEmail` (correct code succeeds, wrong code / expired code / no code found all
  return `ErrVerificationCodeInvalid`), `ResendVerificationCode` (always nil error, generates a
  genuinely different code than a previous call), `RegisterUser`'s new code-generation-and-send path
  (a fake `EmailSender` capturing what was sent, matching this project's existing fake-adapter test
  pattern — e.g. how `internal/adapters/awspolly/generator_test.go` fakes the Polly client rather than
  hitting real AWS), `LoginUser`'s new unverified-account rejection.
- HTTP: the three route handlers' status-code mapping (`201`/`422`/`200`/`403` per the cases above) —
  matching this project's existing handler test pattern (fake `UserRepository`/`EmailSender`, not a real
  Postgres/SES round trip).
- Adapter: `awsses`'s `SendVerificationCode` against a faked SES client, verifying it's called with the
  right recipient/sender/code content — same shape as `awspolly`'s own adapter tests, not a real AWS
  call.
- Mobile: `VerifyEmail` screen isn't component-tested (matches this app's existing convention — no
  screen has component tests); `AuthRepository.ts`'s two new functions get the same kind of unit test
  its siblings already have (mock `fetch`, assert on request shape and response mapping).
- No integration test against real AWS SES for v1 — the founder's own manual AWS-console testing (via
  the actual mobile app, once deployed) is the real-world verification, same as every other AWS-backed
  adapter in this project (Polly, S3) already relies on for its "does this actually work against the
  real service" confidence, per this repo's existing `-m integration`-gated-but-mostly-manually-run
  convention for anything hitting a real external provider.
