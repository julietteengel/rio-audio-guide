# Password reset + localized, branded transactional emails

## Why

Two gaps surfaced while wrapping up the email-verification feature (2026-09-24):

1. There is no password-reset flow at all. `PATCH /me` can change a password, but it requires an
   already-valid auth token — a user who has actually forgotten their password has no recovery path.
2. The verification email is plain text, English-only, and carries none of the app's visual identity
   (colors, fonts) — while the rest of the app already supports fr/en/pt/es via
   `mobile/src/i18n/dictionary.ts`.

Both need the same underlying piece: a code-based, SES-delivered, branded, localized transactional
email. Building that once and using it for both the (existing) verification email and the (new) reset
email is more efficient than building it for one and repeating the work later for the other.

## Scope

In scope: the forgot-password flow (request a code, reset with that code); a shared branded, localized
HTML email template used by both the verification email and the new reset email; retrofitting the
existing verification email onto that shared template and language selection.

Out of scope (explicitly deferred, consistent with gaps already accepted on the verification feature):
rate limiting on any of these routes; session/token revocation on password reset (JWTs stay stateless,
24h TTL, same as today); account lockout after repeated failed reset attempts; storing a user's language
preference server-side — it is deliberately ephemeral, sent fresh on every request that triggers an
email, so an email always reflects the app's *current* language, not whatever language was active at
registration.

## Data model

Two new columns on `users`, mirroring the existing verification-code pair so the two flows can never
clobber each other's in-flight code:

```sql
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS reset_code TEXT,
  ADD COLUMN IF NOT EXISTS reset_code_expires_at TIMESTAMPTZ;
```

## Domain (`internal/domain/user.go`)

No new domain methods. `User.ChangePassword(hash PasswordHash) error` already exists (used today by
`UpdateUserProfile` for an authenticated password change) and is reused as-is to apply the new password
after a successful reset.

## Ports (`internal/ports/`)

`UserRepository` gains two methods mirroring the verification-code pair exactly:

```go
SaveResetCode(ctx context.Context, userID, code string, expiresAt time.Time) error
FindResetCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
```

`EmailSender` changes shape — every method gains a `language` parameter, and a new method is added:

```go
type EmailSender interface {
    SendVerificationCode(ctx context.Context, toEmail, code, language string) error
    SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error
}
```

`language` is one of `"fr" | "en" | "pt" | "es"` (mirrors the mobile app's `Locale` type in
`mobile/src/i18n/dictionary.ts`). Validation/fallback to `"en"` happens inside the adapter, not at the
port boundary — callers pass through whatever the client sent, unvalidated; the adapter is the single
place that decides what an unrecognized or empty value falls back to.

## Application (`internal/application/`)

- `RegisterUser` gains a `language string` parameter, appended at the end of the existing parameter list
  (after `role`) — passed straight through to `emailSender.SendVerificationCode`.
- `ResendVerificationCode` gains the same `language string` parameter, appended at the end.
- New `ForgotPassword(ctx, userRepo, emailSender, email, language string) error`: mirrors
  `ResendVerificationCode` exactly — looks up the user by email, returns `nil` silently for an unknown
  email (anti-enumeration, same posture as `/login` and `/resend-verification-code`), otherwise generates
  a 6-digit code via the existing code-generation helper (reused as-is — it is purpose-agnostic, just "a
  random 6-digit numeric string"; naming it more generically than `generateVerificationCode` is a
  judgment call left to the implementation plan), stores it via `SaveResetCode`, emails it via
  `SendPasswordResetCode`.
- New `ResetPassword(ctx, userRepo, email, code, newPlaintextPassword string) error`: mirrors
  `VerifyEmail`'s validation exactly (find user by email, find reset code, constant-time compare, expiry
  check — all three failure cases collapsing into one `ErrResetCodeInvalid` sentinel, same reasoning as
  `ErrVerificationCodeInvalid`: no operational reason to distinguish "wrong code" from "expired" from "no
  code on file"). On success: bcrypt-hash the new password, `domain.NewPasswordHash`, `user.ChangePassword`
  (identical to `UpdateUserProfile`'s existing password-change block), `userRepo.Save`, then clear the
  reset code the same way `VerifyEmail` clears the verification code — overwrite with an empty code and an
  already-past expiry, so it can never be replayed.
- Applying C1's lesson from the verification feature's final review: if `ResetPassword`'s own
  `userRepo.Save` after `ChangePassword` fails, that is a genuine 500 (unlike `RegisterUser`'s email-send
  failure, there is no "the important part still succeeded" case here — a failed save means the password
  was NOT actually changed).

## Adapter (`internal/adapters/awsses/`)

New file `templates.go`:

- One shared HTML shell, a single Go function `renderEmail(heading, bodyText, code string) string`
  producing a table-based, inline-CSS HTML document: cream background (`#FAF5EE`), a centered white card
  (max-width ~480px, which is what makes a simple single-column email "responsive" without media
  queries), the code displayed large with letter-spacing in terracotta (`#C1592E`), body text in ink
  (`#2B211B`). Custom app fonts (Playfair Display, Inter) are not usable in email clients reliably
  (Outlook in particular ignores `@font-face`); the shell uses a web-safe font stack that approximates
  them (a serif stack for the heading, a system sans-serif stack for body text) instead.
- One copy table keyed by `(emailKind, language)` — `emailKind` is `"verification"` or `"reset"`, four
  languages each — holding the subject/heading/body-text strings for all 8 combinations. An unrecognized
  `language` value falls back to `"en"`'s copy (same fallback point named in the Ports section above).

`sender.go`: both `SendVerificationCode` and the new `SendPasswordResetCode` build a
`types.Body{Html: &types.Content{...}, Text: &types.Content{...}}` — HTML as the primary body, a plain-text
version as the fallback for clients that don't render HTML — instead of today's `Text`-only body, using
`templates.go`'s shared shell plus the copy selected for the given language and email kind.

## HTTP (`internal/adapters/http/`)

- `registerRequest` gains an optional `Language string` JSON field; `registerUser` passes it through to
  `application.RegisterUser`. Empty/missing is fine — the adapter layer above handles the fallback.
- `resendVerificationCodeRequest` gains the same `Language` field.
- Two new routes:
  - `POST /forgot-password` — body `{email, language}` → always `200`, mirroring
    `/resend-verification-code`'s anti-enumeration contract (even the underlying email-send failure case:
    log server-side, still `200`, per the same reasoning the final review applied to
    `/resend-verification-code`).
  - `POST /reset-password` — body `{email, code, newPassword}` → `200` on success, `422` on
    `errors.Is(err, application.ErrResetCodeInvalid)`, `500` on any other error (see the Application
    section's note on why this one path is a genuine 500).
- `NewServer`'s signature is unaffected — both new handlers reuse `s.userRepo`/`s.emailSender`, already
  wired from the verification feature.

## Mobile (`mobile/src/`)

- `AuthRepository.ts`: `register` and `resendVerificationCode` gain a `language: Locale` parameter, sent
  as `language` in the request body. New `forgotPassword(email, language: Locale)` and
  `resetPassword(email, code, newPassword)` functions, same `authFetch` pattern as every other function in
  this file.
- `AuthContext.tsx`: exposes `forgotPassword`/`resetPassword`, both plain pass-throughs to
  `Auth.forgotPassword`/`Auth.resetPassword` — same shape as `verifyEmail`/`resendVerificationCode` already
  are.
- `Auth.tsx`: in login mode only (registering with no account yet makes "forgot password" meaningless), a
  "Forgot password?" link appears between the submit button and the existing
  login/register mode-switch link. It navigates to the new `ForgotPassword` screen. The existing
  `register`/resend call sites are updated to pass `locale` (from `useLocale()`) as the new `language`
  argument — the app already knows its own current locale everywhere else, this is just threading it
  through one more call.
- New `ForgotPassword.tsx`: a single email input, submits to `forgotPassword`, always "succeeds" from the
  UI's point of view, navigates to `ResetPassword` with `{email}` in route params. Same visual shell as
  `Auth.tsx`/`VerifyEmail.tsx` (back button, `SafeAreaView`, theme tokens).
- New `ResetPassword.tsx`: code input + new-password input, a resend link (calls `forgotPassword` again
  for the same email), submit calls `resetPassword` then `login` with the new password (same
  "complete the login the user was trying to do" pattern `VerifyEmail.tsx` already uses) — and on success
  pops **3** screens (`ResetPassword` + `ForgotPassword` + `Auth`) to land back on whatever screen
  originally opened `Auth`. This is the one piece of this spec that most resembles the navigation bug
  found and fixed on `VerifyEmail.tsx` during the verification feature's final review (`navigation.pop(2)`
  there, for a 2-deep stack) — the implementation plan and its review must explicitly verify the pop count
  against the actual stack depth for this screen, not copy the `pop(2)` value by pattern-matching.
- `navigation/types.ts`: `ForgotPassword: undefined` and `ResetPassword: { email: string }` added to
  `AppStackParamList`.
- `AppNavigator.tsx`: both new screens registered with `presentation: "modal"`, alongside `Auth` and
  `VerifyEmail`.
- `dictionary.ts`: new `forgotPassword: {...}` and `resetPassword: {...}` top-level sections (title,
  subtitle, input placeholders, submit CTA, resend link/message, invalid-code message) in all 4 locale
  blocks, plus one new key under the existing `auth` section for the "Forgot password?" link text.

## AWS setup

Nothing beyond what Task 7 (verification feature) already covers — same verified SES sender identity,
same `AmazonSESFullAccess` grant on `rio-cicd`. No new AWS resources; this reuses the existing SES setup
end to end.

## Testing

- Backend: unit tests for `ForgotPassword`/`ResetPassword` mirroring `ResendVerificationCode`'s and
  `VerifyEmail`'s existing test shapes (in-memory fakes, anti-enumeration case, wrong/expired/absent code
  case, happy path, replay-after-success case). HTTP handler tests mirroring the existing
  `/verify-email`/`/resend-verification-code` tests (200/422 cases, always-200 anti-enumeration case, and
  — applying the verification feature's final-review lesson directly — a case asserting
  `/forgot-password` still returns 200 when the underlying email send fails). Adapter test for the
  language/template selection: assert the right language's copy appears in the body for each of the 4
  locales and both email kinds, and that an unrecognized language value falls back to English.
- Mobile: `npx tsc --noEmit` clean; manual verification against a locally running backend, same
  limitation already documented for the verification feature (SES sandbox means no real email arrives
  locally — read the code directly out of Postgres to complete the manual check).
