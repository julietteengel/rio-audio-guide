# Password Reset + Localized Emails Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a forgot-password flow (request a code, reset with it) and replace the plain-text,
English-only verification email with a branded, localized (fr/en/pt/es) HTML email, reusing the same
template for both the verification and the new reset email.

**Architecture:** Mirrors the email-verification feature's own shape exactly — a 6-digit code stored on
`users` (in two new dedicated columns, separate from the verification-code columns so the two flows can
never clobber each other), sent via AWS SES, checked with a constant-time compare and an expiry check
collapsing every failure into one sentinel error. `EmailSender`'s port gains a `language` parameter
threaded from the mobile app's current locale on every request that triggers an email — never stored on
the account, so an email always reflects the app's language at the moment of the request, not whatever
language was active at registration.

**Tech Stack:** Go (hexagonal/DDD backend), Postgres, AWS SES (`github.com/aws/aws-sdk-go-v2/service/sesv2`,
already a dependency), React Native/Expo mobile, `mobile/src/i18n/dictionary.ts`'s existing
`Locale = "fr" | "en" | "pt" | "es"` type.

**Spec:** `docs/superpowers/specs/2026-09-25-password-reset-and-localized-emails-design.md`

## Global Constraints

- Reset code: 6 digits, 15-minute TTL — identical shape to the existing verification code
  (`verificationCodeTTL` in `internal/application/register_user.go`), stored in its own pair of columns
  (`reset_code`, `reset_code_expires_at`), never the verification-code columns.
- Anti-enumeration: `/forgot-password` always returns `200`, whether or not the email belongs to a real
  account, and whether or not the underlying send succeeds — same posture as `/login` and
  `/resend-verification-code`, including under a failed SES send (log server-side, still 200).
- `language` is one of `"fr" | "en" | "pt" | "es"`; an unrecognized or empty value falls back to `"en"`
  inside the `awsses` adapter, never at the port boundary.
- No new AWS resources. No rate limiting (existing accepted gap, tracked separately). No session/token
  revocation on password reset (JWTs stay stateless, 24h TTL, same as today).

---

### Task 1: Reset-code repository support

**Files:**
- Modify: `backend/internal/adapters/postgres/schema.sql`
- Modify: `backend/internal/ports/user_repository.go`
- Modify: `backend/internal/adapters/postgres/user_repository.go`
- Modify: `backend/internal/adapters/postgres/user_repository_test.go`
- Modify: `backend/internal/application/register_user_test.go` (mechanical: add 2 stub methods to the
  existing `fakeUserRepo` so it keeps satisfying `ports.UserRepository` — no behavior change)
- Modify: `backend/internal/adapters/http/user_handler_test.go` (same, for `fakeHTTPUserRepo`)

**Interfaces:**
- Produces: `ports.UserRepository.SaveResetCode(ctx context.Context, userID, code string, expiresAt time.Time) error`,
  `ports.UserRepository.FindResetCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)`
  — consumed by Task 3's `ForgotPassword`/`ResetPassword`.

- [ ] **Step 1: Write the failing integration tests**

Append to `backend/internal/adapters/postgres/user_repository_test.go` (after the existing
`TestUserRepository_EmailVerifiedRoundTrips`, same file, same `//go:build integration` tag already at
the top — do not add a second build tag):

```go
func TestUserRepository_SaveAndFindResetCode(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("reset-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	if err := repo.SaveResetCode(ctx, user.ID(), "654321", expiresAt); err != nil {
		t.Fatalf("save reset code: %v", err)
	}

	code, gotExpiresAt, err := repo.FindResetCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}
	if code != "654321" {
		t.Fatalf("got code %q, want %q", code, "654321")
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("got expiry %v, want %v", gotExpiresAt, expiresAt)
	}
}

func TestUserRepository_SaveResetCode_OverwritesPrevious(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("reset-overwrite+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if err := repo.SaveResetCode(ctx, user.ID(), "111111", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save first code: %v", err)
	}
	if err := repo.SaveResetCode(ctx, user.ID(), "222222", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save second code: %v", err)
	}

	code, _, err := repo.FindResetCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}
	if code != "222222" {
		t.Fatalf("got code %q, want the overwritten %q, not the first one", code, "222222")
	}
}

func TestUserRepository_FindResetCode_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("no-reset-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, _, err := repo.FindResetCode(ctx, user.ID()); err != pgx.ErrNoRows {
		t.Fatalf("got error %v, want pgx.ErrNoRows for a user with no stored reset code", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test -tags integration ./internal/adapters/postgres/... -run "TestUserRepository_SaveAndFindResetCode|TestUserRepository_SaveResetCode_OverwritesPrevious|TestUserRepository_FindResetCode_NotFound" -v`
Expected: FAIL to compile — `repo.SaveResetCode`/`repo.FindResetCode` undefined.

- [ ] **Step 3: Add the columns to `schema.sql`**

In `backend/internal/adapters/postgres/schema.sql`, the `users` table currently ends its code-related
columns with `verification_code_expires_at TIMESTAMPTZ,`. Add two more columns right after it, before
`created_at`:

```sql
CREATE TABLE users (
    id                             TEXT PRIMARY KEY,
    email                          TEXT NOT NULL UNIQUE,
    password_hash                  TEXT NOT NULL,
    role                           TEXT NOT NULL DEFAULT 'user',
    status                         TEXT NOT NULL DEFAULT 'active',
    email_verified                 BOOLEAN NOT NULL DEFAULT false,
    verification_code              TEXT,
    verification_code_expires_at   TIMESTAMPTZ,
    reset_code                     TEXT,
    reset_code_expires_at          TIMESTAMPTZ,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 4: Add the two methods to `ports.UserRepository`**

In `backend/internal/ports/user_repository.go`, add after the existing `FindVerificationCode` method in
the interface:

```go
type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	// SaveVerificationCode overwrites any previously stored code for this
	// user -- a resend replaces, it never accumulates multiple live codes.
	SaveVerificationCode(ctx context.Context, userID, code string, expiresAt time.Time) error
	// FindVerificationCode returns the same not-found error shape as
	// FindByID/FindByEmail (pgx.ErrNoRows, surfaced as-is -- status-code
	// translation happens at the HTTP layer, not here) when no code is
	// currently stored for this user.
	FindVerificationCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
	// SaveResetCode/FindResetCode mirror SaveVerificationCode/FindVerificationCode
	// exactly, but store into reset_code/reset_code_expires_at instead --
	// deliberately separate columns so a password-reset request and an
	// email-verification request for the same account can never overwrite
	// each other's in-flight code.
	SaveResetCode(ctx context.Context, userID, code string, expiresAt time.Time) error
	FindResetCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
}
```

- [ ] **Step 5: Implement both methods in the Postgres adapter**

In `backend/internal/adapters/postgres/user_repository.go`, add after the existing
`FindVerificationCode` method:

```go
// SaveResetCode overwrites any previously stored reset code for this user --
// same overwrite-not-accumulate semantics as SaveVerificationCode.
func (r *UserRepository) SaveResetCode(ctx context.Context, userID, code string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET reset_code = $1, reset_code_expires_at = $2, updated_at = now() WHERE id = $3
	`, code, expiresAt, userID)
	return err
}

// FindResetCode returns pgx.ErrNoRows when the user has no currently-stored
// reset code -- never requested one, or it was already cleared after a
// successful reset. Same nullable-pointer-then-nil-check pattern as
// FindVerificationCode.
func (r *UserRepository) FindResetCode(ctx context.Context, userID string) (string, time.Time, error) {
	var code *string
	var expiresAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT reset_code, reset_code_expires_at FROM users WHERE id = $1
	`, userID).Scan(&code, &expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	if code == nil || expiresAt == nil {
		return "", time.Time{}, pgx.ErrNoRows
	}
	return *code, *expiresAt, nil
}
```

- [ ] **Step 6: Run the integration tests to verify they pass**

Requires a real Postgres reachable via `TEST_DATABASE_URL` (see `testPool` in this package for the exact
env var), with `schema.sql` already applied (re-apply it after Step 3's edit if testing against an
existing local container — `cat internal/adapters/postgres/schema.sql | docker exec -i
<your-postgres-container> psql -U postgres -d postgres`).

Run: `cd backend && go test -tags integration ./internal/adapters/postgres/... -v`
Expected: PASS, all tests in the package (the 3 new ones plus every pre-existing one).

- [ ] **Step 7: Add stub methods to the two existing fakes so the repo keeps compiling**

`ports.UserRepository` now has 2 more methods. Two existing test files each define a fake implementing
this interface; both need the same 2 stub methods added, or `internal/application` and
`internal/adapters/http` will fail to compile the moment this interface changes -- even though neither
package's own tests exercise password reset yet.

In `backend/internal/application/register_user_test.go`, add right after the existing
`FindVerificationCode` method on `fakeUserRepo`:

```go
func (f *fakeUserRepo) SaveResetCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	if f.resetCodes == nil {
		f.resetCodes = map[string]string{}
		f.resetCodeExpiries = map[string]time.Time{}
	}
	f.resetCodes[userID] = code
	f.resetCodeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeUserRepo) FindResetCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.resetCodes[userID]
	if !ok {
		return "", time.Time{}, errors.New("no reset code found")
	}
	return code, f.resetCodeExpiries[userID], nil
}
```

And add the two backing fields to the `fakeUserRepo` struct definition itself (same file, near the top):

```go
type fakeUserRepo struct {
	users             map[string]*domain.User
	byEmail           map[string]string // email -> userID
	codes             map[string]string
	codeExpiries      map[string]time.Time
	resetCodes        map[string]string
	resetCodeExpiries map[string]time.Time
}
```

In `backend/internal/adapters/http/user_handler_test.go`, make the identical addition to
`fakeHTTPUserRepo` -- same two struct fields, same two methods, same lazy-init pattern:

```go
type fakeHTTPUserRepo struct {
	users             map[string]*domain.User
	byEmail           map[string]string
	codes             map[string]string
	codeExpiries      map[string]time.Time
	resetCodes        map[string]string
	resetCodeExpiries map[string]time.Time
}
```

```go
func (f *fakeHTTPUserRepo) SaveResetCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	if f.resetCodes == nil {
		f.resetCodes = map[string]string{}
		f.resetCodeExpiries = map[string]time.Time{}
	}
	f.resetCodes[userID] = code
	f.resetCodeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeHTTPUserRepo) FindResetCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.resetCodes[userID]
	if !ok {
		return "", time.Time{}, errors.New("no reset code found")
	}
	return code, f.resetCodeExpiries[userID], nil
}
```

- [ ] **Step 8: Confirm the whole backend still builds and passes**

Run: `cd backend && go build ./... && go test ./...`
Expected: both clean. (`internal/adapters/postgres` shows "no test files" under this non-`integration`
run, same as always -- its tests are gated behind the `integration` build tag exercised in Step 6.)

- [ ] **Step 9: Commit**

```bash
git add internal/adapters/postgres/schema.sql internal/ports/user_repository.go internal/adapters/postgres/user_repository.go internal/adapters/postgres/user_repository_test.go internal/application/register_user_test.go internal/adapters/http/user_handler_test.go
git commit -m "postgres: reset_code columns, repository methods"
```

---

### Task 2: Localized, branded HTML email templates (adapter layer)

**Files:**
- Modify: `backend/internal/ports/email_sender.go`
- Create: `backend/internal/adapters/awsses/templates.go`
- Create: `backend/internal/adapters/awsses/templates_test.go`
- Modify: `backend/internal/adapters/awsses/sender.go`
- Modify: `backend/internal/adapters/awsses/sender_test.go`

**Interfaces:**
- Produces: `ports.EmailSender.SendVerificationCode(ctx, toEmail, code, language string) error` (signature
  change -- gains `language`), `ports.EmailSender.SendPasswordResetCode(ctx, toEmail, code, language string) error`
  (new). Consumed by Task 3's `RegisterUser`/`ResendVerificationCode`/`ForgotPassword`.
- This task's changes to `ports.EmailSender` will leave `internal/application` and
  `internal/adapters/http` failing to compile (their existing fakes implement the OLD 3-arg
  `SendVerificationCode`, and `RegisterUser`/`ResendVerificationCode` call it with the OLD signature) --
  **this is expected and deferred to Task 3**, exactly like the original email-verification feature's
  Task 4 (AWS SES adapter) left `internal/adapters/http` broken until its own Task 5. Verify success for
  *this* task by building only this package (Step 8 below), not the whole repo.

- [ ] **Step 1: Write the failing template-selection tests**

Create `backend/internal/adapters/awsses/templates_test.go`:

```go
package awsses

import (
	"strings"
	"testing"
)

func TestVerificationCopyFor_KnownLanguage(t *testing.T) {
	for _, lang := range []string{"fr", "en", "pt", "es"} {
		c := verificationCopyFor(lang)
		if c.subject == "" || c.heading == "" || c.body == "" {
			t.Fatalf("language %q: expected non-empty subject/heading/body, got %+v", lang, c)
		}
	}
}

func TestVerificationCopyFor_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	got := verificationCopyFor("de")
	want := verificationCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an unrecognized language, want the English copy %+v", got, want)
	}
}

func TestVerificationCopyFor_EmptyLanguageFallsBackToEnglish(t *testing.T) {
	got := verificationCopyFor("")
	want := verificationCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an empty language, want the English copy %+v", got, want)
	}
}

func TestResetCopyFor_KnownLanguage(t *testing.T) {
	for _, lang := range []string{"fr", "en", "pt", "es"} {
		c := resetCopyFor(lang)
		if c.subject == "" || c.heading == "" || c.body == "" {
			t.Fatalf("language %q: expected non-empty subject/heading/body, got %+v", lang, c)
		}
	}
}

func TestResetCopyFor_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	got := resetCopyFor("de")
	want := resetCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an unrecognized language, want the English copy %+v", got, want)
	}
}

func TestRenderHTML_ContainsHeadingBodyAndCode(t *testing.T) {
	html := renderHTML("My Heading", "My body text.", "123456")
	if !strings.Contains(html, "My Heading") {
		t.Fatal("expected the heading to appear in the rendered HTML")
	}
	if !strings.Contains(html, "My body text.") {
		t.Fatal("expected the body text to appear in the rendered HTML")
	}
	if !strings.Contains(html, "123456") {
		t.Fatal("expected the code to appear in the rendered HTML")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Fatal("expected a full HTML document")
	}
}

func TestRenderText_ContainsBodyAndCode(t *testing.T) {
	text := renderText("My body text.", "123456")
	if !strings.Contains(text, "My body text.") || !strings.Contains(text, "123456") {
		t.Fatalf("got %q, want it to contain both the body text and the code", text)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/awsses/... -run "TestVerificationCopyFor_|TestResetCopyFor_|TestRenderHTML_|TestRenderText_" -v`
Expected: FAIL to compile -- `verificationCopyFor`/`resetCopyFor`/`renderHTML`/`renderText` undefined.

- [ ] **Step 3: Write `templates.go`**

Create `backend/internal/adapters/awsses/templates.go`:

```go
// backend/internal/adapters/awsses/templates.go
package awsses

import "strings"

// emailCopy holds the per-language text for one kind of transactional
// email. body never has the code interpolated into it -- both renderHTML
// and renderText take the code as a separate argument and place it
// themselves, so this struct only ever holds static, hardcoded copy.
type emailCopy struct {
	subject string
	heading string
	body    string
}

var verificationCopy = map[string]emailCopy{
	"en": {
		subject: "Your Memória Carioca verification code",
		heading: "Verify your email",
		body:    "Enter this code in the app to verify your email address. It expires in 15 minutes.",
	},
	"fr": {
		subject: "Votre code de vérification Memória Carioca",
		heading: "Vérifie ton e-mail",
		body:    "Entre ce code dans l'application pour vérifier ton adresse e-mail. Il expire dans 15 minutes.",
	},
	"pt": {
		subject: "Seu código de verificação Memória Carioca",
		heading: "Verifique seu e-mail",
		body:    "Digite este código no aplicativo para verificar seu e-mail. Ele expira em 15 minutos.",
	},
	"es": {
		subject: "Tu código de verificación de Memória Carioca",
		heading: "Verifica tu correo",
		body:    "Ingresa este código en la aplicación para verificar tu correo electrónico. Caduca en 15 minutos.",
	},
}

var resetCopy = map[string]emailCopy{
	"en": {
		subject: "Your Memória Carioca password reset code",
		heading: "Reset your password",
		body:    "Enter this code in the app to reset your password. It expires in 15 minutes. If you didn't request this, you can ignore this email.",
	},
	"fr": {
		subject: "Votre code de réinitialisation Memória Carioca",
		heading: "Réinitialise ton mot de passe",
		body:    "Entre ce code dans l'application pour réinitialiser ton mot de passe. Il expire dans 15 minutes. Si tu n'es pas à l'origine de cette demande, ignore cet e-mail.",
	},
	"pt": {
		subject: "Seu código de redefinição de senha Memória Carioca",
		heading: "Redefina sua senha",
		body:    "Digite este código no aplicativo para redefinir sua senha. Ele expira em 15 minutos. Se você não fez essa solicitação, pode ignorar este e-mail.",
	},
	"es": {
		subject: "Tu código de restablecimiento de Memória Carioca",
		heading: "Restablece tu contraseña",
		body:    "Ingresa este código en la aplicación para restablecer tu contraseña. Caduca en 15 minutos. Si no solicitaste esto, puedes ignorar este correo.",
	},
}

// verificationCopyFor/resetCopyFor fall back to English for an empty or
// unrecognized language -- this is the one place in the whole feature that
// decides the fallback; every caller upstream (application layer, HTTP
// layer) passes the language through unvalidated.
func verificationCopyFor(language string) emailCopy {
	if c, ok := verificationCopy[language]; ok {
		return c
	}
	return verificationCopy["en"]
}

func resetCopyFor(language string) emailCopy {
	if c, ok := resetCopy[language]; ok {
		return c
	}
	return resetCopy["en"]
}

// htmlShell is a table-based, inline-CSS layout -- the only reliable way to
// get consistent rendering across email clients, Outlook desktop in
// particular, which ignores most modern CSS and any <style> block. A single
// column capped at 480px is what makes this "responsive" without needing
// media queries: it just naturally reflows on a narrow (mobile) viewport.
// Brand colors match mobile/src/theme/tokens.ts. The app's custom fonts
// (Playfair Display, Inter) aren't usable in email clients reliably, so
// this uses web-safe stacks that approximate them instead (a serif stack
// for the heading, a system sans-serif stack for body text).
const htmlShell = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  </head>
  <body style="margin:0;padding:0;background-color:#FAF5EE;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#FAF5EE;">
      <tr>
        <td align="center" style="padding:32px 16px;">
          <table role="presentation" width="100%" style="max-width:480px;background-color:#FFFFFF;border-radius:12px;border:1px solid rgba(43,33,27,0.14);" cellpadding="0" cellspacing="0">
            <tr>
              <td style="padding:32px;">
                <h1 style="margin:0 0 16px 0;font-family:Georgia,'Times New Roman',serif;font-size:24px;color:#2B211B;">{{HEADING}}</h1>
                <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 20px 0;">
                  <tr>
                    <td style="background-color:#FAF5EE;border-radius:8px;padding:16px 24px;">
                      <span style="font-family:-apple-system,'Segoe UI',Helvetica,Arial,sans-serif;font-size:28px;letter-spacing:6px;color:#C1592E;font-weight:bold;">{{CODE}}</span>
                    </td>
                  </tr>
                </table>
                <p style="margin:0;font-family:-apple-system,'Segoe UI',Helvetica,Arial,sans-serif;font-size:14px;line-height:21px;color:#6B5D4F;">{{BODY}}</p>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`

// renderHTML/renderText use strings.NewReplacer (not fmt.Sprintf) --
// htmlShell is full of literal "%" characters (e.g. width="100%"), which
// fmt.Sprintf would try to parse as format verbs; token replacement sidesteps
// that entirely.
func renderHTML(heading, body, code string) string {
	r := strings.NewReplacer("{{HEADING}}", heading, "{{CODE}}", code, "{{BODY}}", body)
	return r.Replace(htmlShell)
}

func renderText(body, code string) string {
	return body + "\n\nCode: " + code
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/adapters/awsses/... -run "TestVerificationCopyFor_|TestResetCopyFor_|TestRenderHTML_|TestRenderText_" -v`
Expected: PASS, all 6 tests.

- [ ] **Step 5: Update `ports.EmailSender`**

Replace `backend/internal/ports/email_sender.go` entirely:

```go
package ports

import "context"

type EmailSender interface {
	SendVerificationCode(ctx context.Context, toEmail, code, language string) error
	SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error
}
```

- [ ] **Step 6: Update the existing `sender_test.go` calls to the new signature, and add tests for the new method**

Replace `backend/internal/adapters/awsses/sender_test.go` entirely:

```go
package awsses

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

type fakeSESAPI struct {
	lastInput *sesv2.SendEmailInput
	err       error
}

func (f *fakeSESAPI) SendEmail(_ context.Context, params *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	f.lastInput = params
	if f.err != nil {
		return nil, f.err
	}
	return &sesv2.SendEmailOutput{}, nil
}

func TestSender_SendVerificationCode_SendsFromAndToCorrectAddresses(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "123456", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput == nil {
		t.Fatal("expected SendEmail to have been called")
	}
	if fake.lastInput.FromEmailAddress == nil || *fake.lastInput.FromEmailAddress != "noreply@example.com" {
		t.Fatalf("got FromEmailAddress %v, want noreply@example.com", fake.lastInput.FromEmailAddress)
	}
	if fake.lastInput.Destination == nil || len(fake.lastInput.Destination.ToAddresses) != 1 || fake.lastInput.Destination.ToAddresses[0] != "user@example.com" {
		t.Fatalf("got Destination %+v, want ToAddresses=[user@example.com]", fake.lastInput.Destination)
	}
}

func TestSender_SendVerificationCode_IncludesCodeInHTMLAndTextBody(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "654321", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := fake.lastInput.Content
	if content == nil || content.Simple == nil || content.Simple.Body == nil {
		t.Fatalf("expected a body, got %+v", content)
	}
	if content.Simple.Body.Html == nil || content.Simple.Body.Html.Data == nil || !strings.Contains(*content.Simple.Body.Html.Data, "654321") {
		t.Fatal("expected the HTML body to contain the code")
	}
	if content.Simple.Body.Text == nil || content.Simple.Body.Text.Data == nil || !strings.Contains(*content.Simple.Body.Text.Data, "654321") {
		t.Fatal("expected the plain-text fallback body to contain the code")
	}
}

func TestSender_SendVerificationCode_UsesRequestedLanguage(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111", "fr"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput.Content.Simple.Subject == nil || *fake.lastInput.Content.Simple.Subject.Data != verificationCopy["fr"].subject {
		t.Fatalf("got subject %v, want the French subject %q", fake.lastInput.Content.Simple.Subject, verificationCopy["fr"].subject)
	}
}

func TestSender_SendVerificationCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111", "en"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}

func TestSender_SendPasswordResetCode_SendsWithResetCopy(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendPasswordResetCode(context.Background(), "user@example.com", "987654", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput.Content.Simple.Subject == nil || *fake.lastInput.Content.Simple.Subject.Data != resetCopy["en"].subject {
		t.Fatalf("got subject %v, want the reset-email subject %q", fake.lastInput.Content.Simple.Subject, resetCopy["en"].subject)
	}
	if !strings.Contains(*fake.lastInput.Content.Simple.Body.Text.Data, "987654") {
		t.Fatal("expected the code in the reset email's text body")
	}
}

func TestSender_SendPasswordResetCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendPasswordResetCode(context.Background(), "user@example.com", "987654", "en"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}
```

- [ ] **Step 7: Update `sender.go`**

Replace `backend/internal/adapters/awsses/sender.go` entirely:

```go
// backend/internal/adapters/awsses/sender.go
package awsses

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// sesAPI exposes only the one method used here -- *sesv2.Client satisfies
// it structurally, same pattern already used by internal/adapters/awspolly
// (pollyAPI) for testing without simulating the SDK's own request signing.
type sesAPI interface {
	SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

type Sender struct {
	client sesAPI
	from   string
}

func NewSender(client sesAPI, fromEmail string) *Sender {
	return &Sender{client: client, from: fromEmail}
}

func (s *Sender) SendVerificationCode(ctx context.Context, toEmail, code, language string) error {
	c := verificationCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code)
}

func (s *Sender) SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error {
	c := resetCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code)
}

func (s *Sender) send(ctx context.Context, toEmail, subject, heading, body, code string) error {
	html := renderHTML(heading, body, code)
	text := renderText(body, code)

	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &s.from,
		Destination: &types.Destination{
			ToAddresses: []string{toEmail},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: &subject},
				Body: &types.Body{
					Html: &types.Content{Data: &html},
					Text: &types.Content{Data: &text},
				},
			},
		},
	})
	return err
}
```

- [ ] **Step 8: Confirm this package builds and its own tests pass**

Run: `cd backend && go build ./internal/adapters/awsses/... && go test ./internal/adapters/awsses/... -v`
Expected: clean build, all tests pass (the 6 from Step 1/4 plus the ones from Step 6).

Do **not** run `go build ./...` yet as a pass/fail gate for this task -- `internal/application` and
`internal/adapters/http` are expected to fail to compile until Task 3 updates them (see this task's
Interfaces note above).

- [ ] **Step 9: Commit**

```bash
git add internal/ports/email_sender.go internal/adapters/awsses/
git commit -m "adapters: localized branded HTML email templates, EmailSender language support"
```

---

### Task 3: Application layer — language threading, ForgotPassword, ResetPassword

**Files:**
- Modify: `backend/internal/application/register_user.go`
- Modify: `backend/internal/application/resend_verification_code.go`
- Create: `backend/internal/application/forgot_password.go`
- Create: `backend/internal/application/reset_password.go`
- Modify: `backend/internal/application/register_user_test.go`
- Modify: `backend/internal/application/verify_email_test.go` (mechanical call-site fix only, Step 7)
- Modify: `backend/internal/application/login_user_test.go` (mechanical call-site fix only, Step 7)
- Create: `backend/internal/application/forgot_password_test.go`
- Create: `backend/internal/application/reset_password_test.go`

**Interfaces:**
- Consumes: Task 1's `ports.UserRepository.SaveResetCode`/`FindResetCode`; Task 2's
  `ports.EmailSender.SendVerificationCode(ctx, toEmail, code, language string) error` (new signature) and
  `SendPasswordResetCode(ctx, toEmail, code, language string) error`.
- Produces: `application.RegisterUser(ctx, userRepo, emailSender, email, plaintextPassword, language string, role domain.Role) (*domain.User, error)`
  (signature change -- `language` appended after `plaintextPassword`, before `role`, so the one existing
  HTTP call site is a mechanical edit, not a reorder of unrelated args); `application.ResendVerificationCode(ctx, userRepo, emailSender, email, language string) error`
  (signature change, `language` appended); `application.ForgotPassword(ctx, userRepo, emailSender, email, language string) error`;
  `application.ResetPassword(ctx, userRepo, email, code, newPlaintextPassword string) error`;
  `application.ErrResetCodeInvalid`. Consumed by Task 4's HTTP handlers.
- This task fixes `internal/application`'s compilation (broken by Task 2's port change) but leaves
  `internal/adapters/http` broken until Task 4 -- same deferred-breakage pattern as the previous
  feature's Task 3 → Task 5 boundary.

- [ ] **Step 1: Write the failing tests**

Replace `backend/internal/application/register_user_test.go`'s `fakeEmailSender`/`erroringEmailSender`
types and the two existing `RegisterUser` call sites (everything else in the file -- `fakeUserRepo` and
its methods, including Task 1's `SaveResetCode`/`FindResetCode` stubs -- stays exactly as Task 1 left it):

```go
type fakeEmailSender struct {
	sentTo       []string
	sentCode     []string
	sentLanguage []string
	resetSentTo  []string
}

func (f *fakeEmailSender) SendVerificationCode(_ context.Context, toEmail, code, language string) error {
	f.sentTo = append(f.sentTo, toEmail)
	f.sentCode = append(f.sentCode, code)
	f.sentLanguage = append(f.sentLanguage, language)
	return nil
}

func (f *fakeEmailSender) SendPasswordResetCode(_ context.Context, toEmail, _, _ string) error {
	f.resetSentTo = append(f.resetSentTo, toEmail)
	return nil
}

type erroringEmailSender struct{}

func (erroringEmailSender) SendVerificationCode(context.Context, string, string, string) error {
	return errors.New("ses is down")
}

func (erroringEmailSender) SendPasswordResetCode(context.Context, string, string, string) error {
	return errors.New("ses is down")
}

func TestRegisterUser_GeneratesAndSendsVerificationCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	user, err := RegisterUser(context.Background(), repo, sender, "new@example.com", "password123", "fr", domain.RoleUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.EmailVerified() {
		t.Fatal("a freshly registered user must not be verified yet")
	}

	if len(sender.sentTo) != 1 || sender.sentTo[0] != "new@example.com" {
		t.Fatalf("expected exactly one email sent to new@example.com, got %v", sender.sentTo)
	}
	if sender.sentLanguage[0] != "fr" {
		t.Fatalf("got language %q sent to the adapter, want %q", sender.sentLanguage[0], "fr")
	}
	code := sender.sentCode[0]
	if len(code) != 6 {
		t.Fatalf("got code %q, want exactly 6 digits", code)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Fatalf("got code %q, want only digits", code)
		}
	}

	storedCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("expected a stored code: %v", err)
	}
	if storedCode != code {
		t.Fatalf("stored code %q does not match sent code %q", storedCode, code)
	}
}

func TestRegisterUser_ReturnsErrorWhenEmailSendFails(t *testing.T) {
	repo := newFakeUserRepo()

	_, err := RegisterUser(context.Background(), repo, erroringEmailSender{}, "fail@example.com", "password123", "en", domain.RoleUser)
	if err == nil {
		t.Fatal("expected an error when the email send fails")
	}
	// The account and its code must still exist -- registration isn't
	// rolled back over a transient email-provider failure, the client can
	// use resend.
	found, findErr := repo.FindByEmail(context.Background(), "fail@example.com")
	if findErr != nil {
		t.Fatalf("expected the user to still be persisted despite the send failure: %v", findErr)
	}
	if _, _, err := repo.FindVerificationCode(context.Background(), found.ID()); err != nil {
		t.Fatalf("expected a stored code despite the send failure: %v", err)
	}
}
```

Create `backend/internal/application/forgot_password_test.go`:

```go
package application

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestForgotPassword_GeneratesAndSendsResetCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "forgot@example.com", "password123", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := ForgotPassword(context.Background(), repo, sender, "forgot@example.com", "pt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.resetSentTo) != 1 || sender.resetSentTo[0] != "forgot@example.com" {
		t.Fatalf("expected exactly one reset email sent to forgot@example.com, got %v", sender.resetSentTo)
	}

	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("expected a stored reset code: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("got code %q, want exactly 6 digits", code)
	}
}

func TestForgotPassword_UnknownEmailReturnsNilSilently(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	if err := ForgotPassword(context.Background(), repo, sender, "nobody@example.com", "en"); err != nil {
		t.Fatalf("expected nil error for an unknown email (anti-enumeration), got %v", err)
	}
	if len(sender.resetSentTo) != 0 {
		t.Fatal("expected no reset email sent for an unknown address")
	}
}

func TestForgotPassword_DoesNotTouchVerificationCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "separate-codes@example.com", "password123", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	verificationCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}

	if err := ForgotPassword(context.Background(), repo, sender, "separate-codes@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	stillThere, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code after forgot-password: %v", err)
	}
	if stillThere != verificationCode {
		t.Fatalf("got verification code %q after a password-reset request, want it untouched (%q) -- the two flows must use separate storage", stillThere, verificationCode)
	}
}
```

Create `backend/internal/application/reset_password_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestResetPassword_CorrectCodeChangesPassword(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "reset@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}

	if err := ResetPassword(context.Background(), repo, "reset@example.com", code, "new-password"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindByEmail(context.Background(), "reset@example.com")
	if err != nil {
		t.Fatalf("find by email after reset: %v", err)
	}
	if found.PasswordHash().String() == "" {
		t.Fatal("expected a password hash to be set")
	}
}

func TestResetPassword_WrongCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "wrong-reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "wrong-reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	err := ResetPassword(context.Background(), repo, "wrong-reset@example.com", "000000", "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_ExpiredCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "expired-reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "expired-reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "expired-reset@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}
	// Overwrite with an already-past expiry to simulate a stale code.
	if err := repo.SaveResetCode(context.Background(), user.ID(), code, time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("save expired code: %v", err)
	}

	err = ResetPassword(context.Background(), repo, "expired-reset@example.com", code, "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_UnknownEmailFails(t *testing.T) {
	repo := newFakeUserRepo()

	err := ResetPassword(context.Background(), repo, "nobody@example.com", "123456", "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_ClearsCodeAfterSuccess(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "replay@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "replay@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "replay@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}

	if err := ResetPassword(context.Background(), repo, "replay@example.com", code, "new-password"); err != nil {
		t.Fatalf("first reset: %v", err)
	}

	// Replaying the same code a second time must fail -- it was cleared
	// after the successful reset.
	err = ResetPassword(context.Background(), repo, "replay@example.com", code, "another-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v on replay, want ErrResetCodeInvalid", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/application/... -run "TestRegisterUser_|TestForgotPassword_|TestResetPassword_" -v`
Expected: FAIL to compile -- `RegisterUser` called with the wrong number of arguments, `ForgotPassword`/
`ResetPassword`/`ErrResetCodeInvalid` undefined, `fakeEmailSender` missing `SendPasswordResetCode`.

- [ ] **Step 3: Update `register_user.go`**

In `backend/internal/application/register_user.go`, change the function signature and its one internal
call site:

```go
func RegisterUser(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, plaintextPassword, language string, role domain.Role) (*domain.User, error) {
	emailVO, err := domain.NewEmail(email)
	if err != nil {
		return nil, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	passwordHash, err := domain.NewPasswordHash(string(hashed))
	if err != nil {
		return nil, err
	}

	user := domain.NewUser(emailVO, passwordHash, role)
	if err := userRepo.Save(ctx, user); err != nil {
		return nil, err
	}

	code, err := generateVerificationCode()
	if err != nil {
		return nil, err
	}
	if err := userRepo.SaveVerificationCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return nil, err
	}
	if err := emailSender.SendVerificationCode(ctx, user.Email().String(), code, language); err != nil {
		return user, fmt.Errorf("%w: %v", ErrVerificationEmailNotSent, err)
	}

	return user, nil
}
```

(Only the signature's added `language string` parameter and the `SendVerificationCode` call's added
`language` argument change -- every other line, including the doc comment and `ErrVerificationEmailNotSent`,
stays exactly as-is.)

- [ ] **Step 4: Update `resend_verification_code.go`**

Replace `backend/internal/application/resend_verification_code.go` entirely:

```go
package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// ResendVerificationCode always returns nil, whether or not the email
// belongs to a real account -- mirrors this project's existing
// anti-enumeration stance on /login (ErrInvalidCredentials never
// distinguishes "no such email" from "wrong password"). A resend endpoint
// that reveals which emails have accounts would be a regression from that
// stance.
func ResendVerificationCode(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, language string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil
	}

	code, err := generateVerificationCode()
	if err != nil {
		return err
	}
	if err := userRepo.SaveVerificationCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return err
	}
	return emailSender.SendVerificationCode(ctx, user.Email().String(), code, language)
}
```

- [ ] **Step 5: Write `forgot_password.go`**

Create `backend/internal/application/forgot_password.go`:

```go
package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// ForgotPassword always returns nil, whether or not the email belongs to a
// real account -- same anti-enumeration stance as ResendVerificationCode.
// Reuses generateVerificationCode() as-is: it's purpose-agnostic, just "a
// random 6-digit numeric string" -- there's no reason to duplicate it for
// a second purpose.
func ForgotPassword(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, language string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil
	}

	code, err := generateVerificationCode()
	if err != nil {
		return err
	}
	if err := userRepo.SaveResetCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return err
	}
	return emailSender.SendPasswordResetCode(ctx, user.Email().String(), code, language)
}
```

- [ ] **Step 6: Write `reset_password.go`**

Create `backend/internal/application/reset_password.go`:

```go
package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrResetCodeInvalid covers a wrong code, an expired one, "no code found
// at all", and "no such account" -- deliberately not distinguished, same
// reasoning as ErrVerificationCodeInvalid: there's no operational reason to
// tell a client which of the four happened, and collapsing "no such
// account" into the same error here (rather than a separate case) keeps
// this endpoint from being a second anti-enumeration surface to get wrong.
var ErrResetCodeInvalid = errors.New("application: invalid or expired reset code")

// ResetPassword mirrors VerifyEmail's validation shape exactly, but applies
// a new password instead of marking the account verified. Unlike
// RegisterUser's email-send failure (where the account still exists and
// works), a failed userRepo.Save here after ChangePassword is a genuine
// error to the caller -- the password was NOT actually changed, there is no
// "the important part still succeeded" case.
func ResetPassword(ctx context.Context, userRepo ports.UserRepository, email, code, newPlaintextPassword string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrResetCodeInvalid
	}

	storedCode, expiresAt, err := userRepo.FindResetCode(ctx, user.ID())
	if err != nil {
		return ErrResetCodeInvalid
	}

	if subtle.ConstantTimeCompare([]byte(storedCode), []byte(code)) != 1 {
		return ErrResetCodeInvalid
	}
	if time.Now().After(expiresAt) {
		return ErrResetCodeInvalid
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPlaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	passwordHash, err := domain.NewPasswordHash(string(hashed))
	if err != nil {
		return err
	}
	if err := user.ChangePassword(passwordHash); err != nil {
		return err
	}
	if err := userRepo.Save(ctx, user); err != nil {
		return err
	}
	// Clear the code so it can't be replayed -- same technique VerifyEmail
	// uses: an already-past expiry makes any future FindResetCode/compare
	// fail regardless of what value is stored.
	return userRepo.SaveResetCode(ctx, user.ID(), "", time.Now().Add(-1*time.Hour))
}
```

- [ ] **Step 7: Fix the two other files that still call the old signatures**

`RegisterUser`'s and `ResendVerificationCode`'s signature changes (Steps 3-4) break every call site in
the package, not just the ones in `register_user_test.go`. Two other existing test files call them too
and must be updated, or the whole `internal/application` package fails to compile:

In `backend/internal/application/verify_email_test.go`, every `RegisterUser(...)` call gains a
`"en"` argument before the trailing `domain.RoleUser`, and the one `ResendVerificationCode(...)` calls
gain a trailing `"en"` argument:

```go
	user, err := RegisterUser(context.Background(), repo, sender, "verify@example.com", "password123", "en", domain.RoleUser)
```
```go
	if _, err := RegisterUser(context.Background(), repo, sender, "wrong-code@example.com", "password123", "en", domain.RoleUser); err != nil {
```
```go
	user, err := RegisterUser(context.Background(), repo, sender, "expired@example.com", "password123", "en", domain.RoleUser)
```
```go
	user, err := RegisterUser(context.Background(), repo, sender, "resend@example.com", "password123", "en", domain.RoleUser)
```
```go
	if err := ResendVerificationCode(context.Background(), repo, sender, "resend@example.com", "en"); err != nil {
```
```go
	if err := ResendVerificationCode(context.Background(), repo, sender, "nobody@example.com", "en"); err != nil {
```

In `backend/internal/application/login_user_test.go`, both `RegisterUser(...)` calls gain the same
trailing `"en"` argument before `domain.RoleUser`:

```go
	if _, err := RegisterUser(context.Background(), repo, sender, "unverified@example.com", "password123", "en", domain.RoleUser); err != nil {
```
```go
	if _, err := RegisterUser(context.Background(), repo, sender, "will-verify@example.com", "password123", "en", domain.RoleUser); err != nil {
```

Every other line in both files (assertions, comments, the rest of each test body) is unchanged -- this
is purely a call-site argument addition, no behavior or expectation changes.

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/application/... -run "TestRegisterUser_|TestForgotPassword_|TestResetPassword_" -v`
Expected: PASS, all tests across the three new/modified files.

- [ ] **Step 9: Run the full application package test suite to confirm nothing else broke**

Run: `cd backend && go test ./internal/application/... -v`
Expected: PASS, every test in the package -- the ones from this task, plus `TestVerifyEmail_*`,
`TestResendVerificationCode_*`, and `TestLoginUser_*` (updated in Step 7, behavior unchanged), plus every
other pre-existing test untouched by this task.

- [ ] **Step 10: Commit**

```bash
git add internal/application/register_user.go internal/application/resend_verification_code.go internal/application/forgot_password.go internal/application/reset_password.go internal/application/register_user_test.go internal/application/verify_email_test.go internal/application/login_user_test.go internal/application/forgot_password_test.go internal/application/reset_password_test.go
git commit -m "application: language-aware emails, forgot/reset password"
```

---

### Task 4: HTTP routes — language threading, /forgot-password, /reset-password

**Files:**
- Modify: `backend/internal/adapters/http/user_handler.go`
- Modify: `backend/internal/adapters/http/server.go`
- Modify: `backend/internal/adapters/http/user_handler_test.go`

**Interfaces:**
- Consumes: Task 3's `application.RegisterUser` (new signature), `ResendVerificationCode` (new
  signature), `ForgotPassword`, `ResetPassword`, `ErrResetCodeInvalid`.
- Produces: `POST /forgot-password` (`{email, language}` → always `200`), `POST /reset-password`
  (`{email, code, newPassword}` → `200`/`422`/`500`). Consumed by Task 5's mobile `AuthRepository.ts`.
- This task fixes the last remaining compile break -- after this task, `go build ./...` and
  `go test ./...` are green across the whole backend again.

- [ ] **Step 1: Write the failing HTTP handler tests**

Replace `backend/internal/adapters/http/user_handler_test.go`'s `fakeHTTPEmailSender` type and every
existing test that calls `SendVerificationCode`/registers/resends through the handlers (everything else
in the file -- `fakeHTTPUserRepo` and its methods, `newTestServerForUserHandlers`,
`fakeTokenIssuerForHTTP` -- stays exactly as Task 1 left it):

```go
type fakeHTTPEmailSender struct {
	lastCode        string
	lastLanguage    string
	lastResetCode   string
	lastResetToAddr string
	err             error
}

func (f *fakeHTTPEmailSender) SendVerificationCode(_ context.Context, _, code, language string) error {
	f.lastCode = code
	f.lastLanguage = language
	if f.err != nil {
		return f.err
	}
	return nil
}

func (f *fakeHTTPEmailSender) SendPasswordResetCode(_ context.Context, toEmail, code, _ string) error {
	f.lastResetToAddr = toEmail
	f.lastResetCode = code
	if f.err != nil {
		return f.err
	}
	return nil
}
```

Add these new tests to the same file (placed after the existing `TestRegisterHandler_...` tests):

```go
func TestForgotPasswordHandler_AlwaysReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "nobody@example.com", "language": "en"})
	req := httptest.NewRequest("POST", "/forgot-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200 even for an unknown email, body: %s", rec.Code, rec.Body.String())
	}
}

func TestForgotPasswordHandler_Returns200EvenWhenSendFails(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	registerSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, registerSender, "forgot-fails@example.com", "password123", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	emailSender := &fakeHTTPEmailSender{err: errors.New("ses rejected the request")}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "forgot-fails@example.com", "language": "en"})
	req := httptest.NewRequest("POST", "/forgot-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200 even though the reset email's send failed, body: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandler_CorrectCodeReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "reset-handler@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := application.ForgotPassword(context.Background(), userRepo, emailSender, "reset-handler@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "reset-handler@example.com", "code": emailSender.lastResetCode, "newPassword": "new-password"})
	req := httptest.NewRequest("POST", "/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandler_WrongCodeReturns422(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "reset-handler2@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := application.ForgotPassword(context.Background(), userRepo, emailSender, "reset-handler2@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "reset-handler2@example.com", "code": "000000", "newPassword": "new-password"})
	req := httptest.NewRequest("POST", "/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 422 {
		t.Fatalf("got status %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/http/... -run "TestForgotPasswordHandler_|TestResetPasswordHandler_" -v`
Expected: FAIL to compile -- this package has been left broken since Task 2 changed `ports.EmailSender`
(deferred on purpose, see this task's Interfaces note). Step 1 above fixes `fakeHTTPEmailSender`'s own
signature, but `user_handler.go`'s existing calls to `application.RegisterUser`/`ResendVerificationCode`
still use the pre-Task-3 argument counts, and the `/forgot-password`/`/reset-password` routes and
handlers don't exist yet -- Step 3 below fixes all of it at once.

- [ ] **Step 3: Update `user_handler.go`**

In `backend/internal/adapters/http/user_handler.go`, update `registerRequest` and `registerUser`:

```go
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Language string `json:"language"`
}
```

```go
func (s *Server) registerUser(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	user, err := application.RegisterUser(c.Request().Context(), s.userRepo, s.emailSender, req.Email, req.Password, req.Language, domain.RoleUser)
	if err != nil {
		if !errors.Is(err, application.ErrVerificationEmailNotSent) {
			return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
		}
		log.Printf("registerUser: verification email failed to send for user %s: %v", user.ID(), err)
	}
	return c.JSON(http.StatusCreated, userResponse{ID: user.ID(), Email: user.Email().String(), Role: user.Role().String()})
}
```

Update `resendVerificationCodeRequest` and `resendVerificationCode`:

```go
type resendVerificationCodeRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
}
```

```go
func (s *Server) resendVerificationCode(c echo.Context) error {
	var req resendVerificationCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if err := application.ResendVerificationCode(c.Request().Context(), s.userRepo, s.emailSender, req.Email, req.Language); err != nil {
		log.Printf("resendVerificationCode: %v", err)
	}
	return c.JSON(http.StatusOK, echo.Map{})
}
```

Add the two new handlers and their request types, placed after `resendVerificationCode`:

```go
type forgotPasswordRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
}

// forgotPassword always returns 200, whether or not the email belongs to a
// real account, and whether or not the send itself succeeded -- same
// anti-enumeration reasoning as resendVerificationCode's own comment.
func (s *Server) forgotPassword(c echo.Context) error {
	var req forgotPasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if err := application.ForgotPassword(c.Request().Context(), s.userRepo, s.emailSender, req.Email, req.Language); err != nil {
		log.Printf("forgotPassword: %v", err)
	}
	return c.JSON(http.StatusOK, echo.Map{})
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"newPassword"`
}

func (s *Server) resetPassword(c echo.Context) error {
	var req resetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if err := application.ResetPassword(c.Request().Context(), s.userRepo, req.Email, req.Code, req.NewPassword); err != nil {
		if errors.Is(err, application.ErrResetCodeInvalid) {
			return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": "invalid or expired code"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{})
}
```

- [ ] **Step 4: Register the two new routes**

In `backend/internal/adapters/http/server.go`, add right after the existing
`s.echo.POST("/resend-verification-code", s.resendVerificationCode)` line:

```go
	s.echo.POST("/register", s.registerUser)
	s.echo.POST("/login", s.login)
	s.echo.POST("/verify-email", s.verifyEmail)
	s.echo.POST("/resend-verification-code", s.resendVerificationCode)
	s.echo.POST("/forgot-password", s.forgotPassword)
	s.echo.POST("/reset-password", s.resetPassword)
	s.echo.POST("/logout", s.logout, auth)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/adapters/http/... -v`
Expected: PASS, every test in the package (the new ones plus every pre-existing one -- no regressions).

- [ ] **Step 6: Build the whole backend to confirm everything compiles together**

Run: `cd backend && go build ./...`
Expected: no output, exit 0. This is the first point since Task 2 where the whole backend is expected to
compile clean again.

- [ ] **Step 7: Run the full backend test suite**

Run: `cd backend && go test ./...`
Expected: PASS across every package.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/http/user_handler.go internal/adapters/http/server.go internal/adapters/http/user_handler_test.go
git commit -m "http: /forgot-password, /reset-password routes; language on register/resend"
```

---

### Task 5: Mobile — forgot password flow, language threading

**Files:**
- Modify: `mobile/src/data/AuthRepository.ts`
- Modify: `mobile/src/auth/AuthContext.tsx`
- Modify: `mobile/src/screens/Auth.tsx`
- Create: `mobile/src/screens/ForgotPassword.tsx`
- Create: `mobile/src/screens/ResetPassword.tsx`
- Modify: `mobile/src/navigation/types.ts`
- Modify: `mobile/src/navigation/AppNavigator.tsx`
- Modify: `mobile/src/i18n/dictionary.ts`

**Interfaces:**
- Consumes: Task 4's `POST /forgot-password` (`{email, language}` → `200`), `POST /reset-password`
  (`{email, code, newPassword}` → `200`/`422`), `POST /register` and `POST /resend-verification-code`'s
  new optional `language` field.
- Produces: `AuthRepository.forgotPassword(email, language: Locale): Promise<void>`,
  `AuthRepository.resetPassword(email, code, newPassword): Promise<void>`; `AppStackParamList`'s new
  `ForgotPassword: undefined` and `ResetPassword: { email: string }` routes.

- [ ] **Step 1: Update `AuthRepository.ts`**

In `mobile/src/data/AuthRepository.ts`, add the `Locale` import at the top, alongside the existing
imports:

```ts
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";
```

Change the `register` and `resendVerificationCode` functions to accept and send a `language: Locale`
parameter:

```ts
export async function register(email: string, password: string, language: Locale): Promise<AuthUser> {
  const user = await authFetch<AuthUser>("/register", { method: "POST", body: { email, password, language } });
  if (!user) throw new AuthApiError("empty response", 500);
  return user;
}
```

```ts
export async function resendVerificationCode(email: string, language: Locale): Promise<void> {
  await authFetch<Record<string, never>>("/resend-verification-code", { method: "POST", body: { email, language } });
}
```

Add two new functions at the end of the file:

```ts
export async function forgotPassword(email: string, language: Locale): Promise<void> {
  await authFetch<Record<string, never>>("/forgot-password", { method: "POST", body: { email, language } });
}

export async function resetPassword(email: string, code: string, newPassword: string): Promise<void> {
  await authFetch<Record<string, never>>("/reset-password", { method: "POST", body: { email, code, newPassword } });
}
```

- [ ] **Step 2: Update `AuthContext.tsx`**

In `mobile/src/auth/AuthContext.tsx`, update the `AuthContextValue` type: `register` gains a `language:
Locale` parameter, and two new functions are added.

```ts
import type { Locale } from "../i18n/dictionary";
```

(added to the top import block, alongside the existing `AuthUser` import)

```ts
type AuthContextValue = {
  user: AuthUser | null;
  token: string | null;
  isLoggedIn: boolean;
  isLoading: boolean;
  register: (email: string, password: string, language: Locale) => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  updateProfile: (changes: { email?: string; password?: string }) => Promise<void>;
  deleteAccount: () => Promise<void>;
  verifyEmail: (email: string, code: string) => Promise<void>;
  resendVerificationCode: (email: string, language: Locale) => Promise<void>;
  forgotPassword: (email: string, language: Locale) => Promise<void>;
  resetPassword: (email: string, code: string, newPassword: string) => Promise<void>;
};
```

Update `registerFn`'s signature (its body is unchanged -- still no auto-login, same reasoning as
today's comment):

```ts
  async function registerFn(email: string, password: string, language: Locale) {
    await Auth.register(email, password, language);
    // Deliberately no auto-login here anymore: a freshly registered
    // account isn't verified yet, and /login now rejects with 403 until it
    // is. AuthScreen.submit() navigates to VerifyEmail next, which calls
    // login() itself once the code is confirmed.
  }
```

Update the `useMemo` value object to expose the two new functions and pass `resendVerificationCode`
through as-is (it already takes a `language` param on the `Auth.*` side, so no wrapper function is
needed -- same as `verifyEmail` today):

```ts
  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      isLoggedIn: user !== null,
      isLoading,
      register: registerFn,
      login: loginFn,
      logout: logoutFn,
      updateProfile: updateProfileFn,
      deleteAccount: deleteAccountFn,
      verifyEmail: Auth.verifyEmail,
      resendVerificationCode: Auth.resendVerificationCode,
      forgotPassword: Auth.forgotPassword,
      resetPassword: Auth.resetPassword,
    }),
    [user, token, isLoading],
  );
```

- [ ] **Step 3: Update `VerifyEmail.tsx`'s resend call site**

`resendVerificationCode` now requires a `language` argument. In `mobile/src/screens/VerifyEmail.tsx`,
add the `useLocale` import and pass `locale` through both call sites (the mount effect and the `resend`
function):

```ts
import { useLocale } from "../i18n/LocaleContext";
```

(if not already imported in this file -- it already is, since `t` comes from `useLocale()`; this step
is a no-op if so, skip straight to the two call-site edits below)

```ts
  const { t, locale } = useLocale();
```

```ts
  useEffect(() => {
    if (codeAlreadySent) return;
    let cancelled = false;
    resendVerificationCode(email, locale)
      .catch(() => {
        if (!cancelled) setError(t.auth.networkError);
      })
      .finally(() => {
        if (!cancelled) setInitializing(false);
      });
    return () => {
      cancelled = true;
    };
  }, [codeAlreadySent, email, locale, resendVerificationCode, t]);
```

```ts
  async function resend() {
    setResendMessage(null);
    setResending(true);
    try {
      await resendVerificationCode(email, locale);
      setResendMessage(t.verifyEmail.resendSent);
    } catch {
      setError(t.auth.networkError);
    } finally {
      setResending(false);
    }
  }
```

- [ ] **Step 4: Update `Auth.tsx`**

In `mobile/src/screens/Auth.tsx`, destructure `locale` from `useLocale()` and pass it into `register`;
add a "Forgot password?" link (login mode only) navigating to the new `ForgotPassword` screen.

```tsx
  const { t, locale } = useLocale();
```

```tsx
  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      if (mode === "login") {
        await login(email.trim(), password);
        navigation.goBack();
      } else {
        await register(email.trim(), password, locale);
        navigation.navigate("VerifyEmail", { email: email.trim(), password, codeAlreadySent: true });
      }
    } catch (err) {
      if (err instanceof AuthApiError && err.status === 403) {
        navigation.navigate("VerifyEmail", { email: email.trim(), password, codeAlreadySent: false });
      } else {
        setError(err instanceof AuthApiError ? t.auth.genericError : t.auth.networkError);
      }
    } finally {
      setSubmitting(false);
    }
  }
```

(Only the `register(...)` call's new `locale` argument changes in this function -- everything else,
including the whole `catch` block, is unchanged.)

Add the "Forgot password?" link, in login mode only, between the submit button and the existing
`switchLink`:

```tsx
        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? (
            <ActivityIndicator color={colors.cream} />
          ) : (
            <Text style={styles.btnText}>{mode === "login" ? t.auth.loginCta : t.auth.registerCta}</Text>
          )}
        </Pressable>

        {mode === "login" ? (
          <Pressable style={styles.forgotPasswordLink} onPress={() => navigation.navigate("ForgotPassword")}>
            <Text style={styles.forgotPasswordLinkText}>{t.auth.forgotPasswordLink}</Text>
          </Pressable>
        ) : null}

        <Pressable
          style={styles.switchLink}
          onPress={() => {
            setError(null);
            setMode((m) => (m === "login" ? "register" : "login"));
          }}
        >
          <Text style={styles.switchLinkText}>
            {mode === "login" ? t.auth.switchToRegister : t.auth.switchToLogin}
          </Text>
        </Pressable>
```

Add the two new styles to the `StyleSheet.create` call, right after `btnText`:

```tsx
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
  forgotPasswordLink: { paddingTop: 16, alignItems: "center" },
  forgotPasswordLinkText: { fontFamily: fonts.bodySemiBold, fontSize: 13, color: colors.inkSoft },
  switchLink: { paddingVertical: 16, alignItems: "center" },
```

- [ ] **Step 5: Add the new dictionary keys (all 4 locales)**

In `mobile/src/i18n/dictionary.ts`, each of the 4 `auth: { ... }` blocks gains one new key right after
the existing `emailNotVerified` line:

- `fr`: `forgotPasswordLink: "Mot de passe oublié ?",`
- `en`: `forgotPasswordLink: "Forgot password?",`
- `pt`: `forgotPasswordLink: "Esqueceu a senha?",`
- `es`: `forgotPasswordLink: "¿Olvidaste tu contraseña?",`

Add two new top-level sections to each of the 4 locale blocks, placed next to the existing
`verifyEmail: { ... }` section, same nesting level (not inside `auth` or `verifyEmail`):

`fr`:
```ts
    forgotPassword: {
      title: "Mot de passe oublié",
      subtitle: "Entre ton adresse e-mail, on t'enverra un code pour réinitialiser ton mot de passe.",
      submitCta: "Envoyer le code",
    },
    resetPassword: {
      title: "Réinitialise ton mot de passe",
      subtitle: "On t'a envoyé un code à 6 chiffres à {email}. Entre-le avec ton nouveau mot de passe.",
      codePlaceholder: "123456",
      newPasswordPlaceholder: "Nouveau mot de passe",
      submitCta: "Réinitialiser",
      resendLink: "Renvoyer le code",
      resendSent: "Code renvoyé.",
      invalidCode: "Code invalide ou expiré.",
    },
```

`en`:
```ts
    forgotPassword: {
      title: "Forgot password",
      subtitle: "Enter your email address and we'll send you a code to reset your password.",
      submitCta: "Send code",
    },
    resetPassword: {
      title: "Reset your password",
      subtitle: "We sent a 6-digit code to {email}. Enter it along with your new password.",
      codePlaceholder: "123456",
      newPasswordPlaceholder: "New password",
      submitCta: "Reset password",
      resendLink: "Resend code",
      resendSent: "Code resent.",
      invalidCode: "Invalid or expired code.",
    },
```

`pt`:
```ts
    forgotPassword: {
      title: "Esqueceu a senha",
      subtitle: "Digite seu e-mail e enviaremos um código para redefinir sua senha.",
      submitCta: "Enviar código",
    },
    resetPassword: {
      title: "Redefina sua senha",
      subtitle: "Enviamos um código de 6 dígitos para {email}. Digite-o com sua nova senha.",
      codePlaceholder: "123456",
      newPasswordPlaceholder: "Nova senha",
      submitCta: "Redefinir",
      resendLink: "Reenviar código",
      resendSent: "Código reenviado.",
      invalidCode: "Código inválido ou expirado.",
    },
```

`es`:
```ts
    forgotPassword: {
      title: "¿Olvidaste tu contraseña?",
      subtitle: "Ingresa tu correo y te enviaremos un código para restablecer tu contraseña.",
      submitCta: "Enviar código",
    },
    resetPassword: {
      title: "Restablece tu contraseña",
      subtitle: "Te enviamos un código de 6 dígitos a {email}. Ingrésalo junto con tu nueva contraseña.",
      codePlaceholder: "123456",
      newPasswordPlaceholder: "Nueva contraseña",
      submitCta: "Restablecer",
      resendLink: "Reenviar código",
      resendSent: "Código reenviado.",
      invalidCode: "Código inválido o expirado.",
    },
```

- [ ] **Step 6: Add the two new routes to `navigation/types.ts`**

```ts
export type AppStackParamList = {
  Map: undefined;
  PlaceDetail: { placeId: string };
  Assistant: { placeId: string };
  Settings: undefined;
  Auth: undefined;
  EditProfile: undefined;
  ItinerariesList: undefined;
  ItineraryChat: undefined;
  ItineraryDetail: { itineraryId: string };
  VerifyEmail: { email: string; password: string; codeAlreadySent: boolean };
  ForgotPassword: undefined;
  ResetPassword: { email: string };
};
```

- [ ] **Step 7: Write `ForgotPassword.tsx`**

Follow the exact same screen shell already established by `Auth.tsx`/`VerifyEmail.tsx` (back button,
`SafeAreaView`, theme tokens):

```tsx
// mobile/src/screens/ForgotPassword.tsx
import React, { useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ForgotPassword">;

export function ForgotPasswordScreen({ navigation }: Props) {
  const { t, locale } = useLocale();
  const { forgotPassword } = useAuth();
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit() {
    setSubmitting(true);
    try {
      // Always "succeeds" from this screen's point of view -- the backend
      // never reveals whether the email belongs to a real account
      // (anti-enumeration), so there is nothing to branch on here.
      await forgotPassword(email.trim(), locale);
    } catch {
      // A network failure still moves on to ResetPassword: the "resend"
      // link there covers retrying the actual send.
    } finally {
      setSubmitting(false);
      navigation.navigate("ResetPassword", { email: email.trim() });
    }
  }

  const canSubmit = email.trim().length > 0 && !submitting;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <View style={styles.form}>
        <Text style={styles.title}>{t.forgotPassword.title}</Text>
        <Text style={styles.subtitle}>{t.forgotPassword.subtitle}</Text>

        <TextInput
          style={styles.input}
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          placeholder="vous@exemple.com"
          placeholderTextColor={colors.inkFaint}
          editable={!submitting}
        />

        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.forgotPassword.submitCta}</Text>}
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  form: { marginHorizontal: spacing.xl, marginTop: spacing.xl },
  title: { fontFamily: fonts.display, fontSize: 26, color: colors.ink, marginBottom: 10 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, marginBottom: spacing.lg },
  input: {
    fontFamily: fonts.body,
    fontSize: 15,
    color: colors.ink,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 12,
    paddingHorizontal: 14,
    marginBottom: spacing.md,
  },
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
});
```

- [ ] **Step 8: Write `ResetPassword.tsx`**

```tsx
// mobile/src/screens/ResetPassword.tsx
import React, { useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { AuthApiError } from "../data/AuthRepository";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ResetPassword">;

export function ResetPasswordScreen({ route, navigation }: Props) {
  const { email } = route.params;
  const { t, locale } = useLocale();
  const { resetPassword, forgotPassword, login } = useAuth();
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendMessage, setResendMessage] = useState<string | null>(null);

  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      await resetPassword(email, code.trim(), newPassword);
      // Complete the login with the password the user just set, same
      // "finish what they came here to do" pattern VerifyEmail.tsx uses.
      await login(email, newPassword);
      // ResetPassword -> ForgotPassword -> Auth is 3 screens deep on
      // AppNavigator's own stack (all three are plain sibling screens
      // there, not nested navigators -- see VerifyEmail.tsx's comment on
      // the same stack for why getParent() would be wrong here too), so
      // popping 3 lands back on whatever screen originally opened Auth.
      navigation.pop(3);
    } catch (err) {
      setError(err instanceof AuthApiError ? t.resetPassword.invalidCode : t.auth.networkError);
    } finally {
      setSubmitting(false);
    }
  }

  async function resend() {
    setResendMessage(null);
    setResending(true);
    try {
      await forgotPassword(email, locale);
      setResendMessage(t.resetPassword.resendSent);
    } catch {
      setError(t.auth.networkError);
    } finally {
      setResending(false);
    }
  }

  const canSubmit = code.trim().length > 0 && newPassword.length > 0 && !submitting && !resending;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <View style={styles.form}>
        <Text style={styles.title}>{t.resetPassword.title}</Text>
        <Text style={styles.subtitle}>{t.resetPassword.subtitle.replace("{email}", email)}</Text>

        <TextInput
          style={styles.codeInput}
          value={code}
          onChangeText={setCode}
          keyboardType="number-pad"
          maxLength={6}
          placeholder={t.resetPassword.codePlaceholder}
          placeholderTextColor={colors.inkFaint}
          editable={!submitting && !resending}
        />

        <TextInput
          style={styles.input}
          value={newPassword}
          onChangeText={setNewPassword}
          secureTextEntry
          autoCapitalize="none"
          placeholder={t.resetPassword.newPasswordPlaceholder}
          placeholderTextColor={colors.inkFaint}
          editable={!submitting && !resending}
        />

        {error ? <Text style={styles.error}>{error}</Text> : null}
        {resendMessage ? <Text style={styles.resendMessage}>{resendMessage}</Text> : null}

        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.resetPassword.submitCta}</Text>}
        </Pressable>

        <Pressable style={styles.resendLinkWrap} onPress={resend} disabled={resending || submitting}>
          <Text style={styles.resendLinkText}>{t.resetPassword.resendLink}</Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  form: { marginHorizontal: spacing.xl, marginTop: spacing.xl },
  title: { fontFamily: fonts.display, fontSize: 26, color: colors.ink, marginBottom: 10 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, marginBottom: spacing.lg },
  codeInput: {
    fontFamily: fonts.body,
    fontSize: 20,
    letterSpacing: 4,
    textAlign: "center",
    color: colors.ink,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 14,
    marginBottom: spacing.md,
  },
  input: {
    fontFamily: fonts.body,
    fontSize: 15,
    color: colors.ink,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 12,
    paddingHorizontal: 14,
    marginBottom: spacing.md,
  },
  error: { fontFamily: fonts.body, fontSize: 13, color: colors.terracottaDark, marginBottom: spacing.sm },
  resendMessage: { fontFamily: fonts.body, fontSize: 13, color: colors.inkSoft, marginBottom: spacing.sm },
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
  resendLinkWrap: { paddingVertical: 16, alignItems: "center" },
  resendLinkText: { fontFamily: fonts.bodySemiBold, fontSize: 14, color: colors.terracotta },
});
```

- [ ] **Step 9: Register the two new screens in `AppNavigator.tsx`**

```tsx
import { ForgotPasswordScreen } from "../screens/ForgotPassword";
import { ResetPasswordScreen } from "../screens/ResetPassword";
```

(added to the import block, alongside the other screen imports)

```tsx
      <Stack.Screen name="Auth" component={AuthScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="VerifyEmail" component={VerifyEmailScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ForgotPassword" component={ForgotPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ResetPassword" component={ResetPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="EditProfile" component={EditProfileScreen} />
```

(added right after `VerifyEmail`'s registration, same `presentation: "modal"` option -- reached only
from within that same modal flow)

- [ ] **Step 10: Typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 11: Manual verification**

With the backend running locally and Task 4's routes live: from the login screen, tap "Forgot
password?", enter a registered test account's email, confirm it lands on the reset-code screen. Since a
real local backend has no real SES access configured (same limitation documented for the verification
feature), read the code directly out of the local Postgres `users` table's `reset_code` column
(`docker exec <your-postgres-container> psql -U postgres -d postgres -c "SELECT reset_code FROM users
WHERE email = '<the test email>';"`), enter it with a new password, and confirm it logs in and returns to
the app's main screen. Separately, confirm the "Resend code" link on this screen still works after that.

- [ ] **Step 12: Commit**

```bash
git add src/data/AuthRepository.ts src/auth/AuthContext.tsx src/screens/Auth.tsx src/screens/VerifyEmail.tsx src/screens/ForgotPassword.tsx src/screens/ResetPassword.tsx src/navigation/types.ts src/navigation/AppNavigator.tsx src/i18n/dictionary.ts
git commit -m "mobile: forgot password flow, localized email language"
```

---

### Task 6: Apply schema change to the deployed database (manual, founder's own work)

**Files:** none (this task is one command against the already-deployed EC2 database — no repository
files change).

**Interfaces:** none.

- [ ] **Step 1: Apply the schema change**

Same pattern as the email-verification feature's own deployed-database step: via EC2 Instance Connect,
run this **before** redeploying the API with this branch's code (the new binary's SQL names
`reset_code`/`reset_code_expires_at` explicitly, so redeploying first would 500 every user route in the
window between the two steps — identical reasoning already documented for the verification feature's
migration):

```bash
sudo docker exec rio-backend-postgres-1 psql -U postgres -d postgres -c "
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS reset_code TEXT,
  ADD COLUMN IF NOT EXISTS reset_code_expires_at TIMESTAMPTZ;
"
```

- [ ] **Step 2: Redeploy**

Same command already used for every prior redeploy on this instance:

```bash
cd ~/rio-backend
sudo docker compose -f docker-compose.prod.yml up -d --build api
```
