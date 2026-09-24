# Email Verification (AWS SES) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require a 6-digit emailed code before a new account can log in, sent via AWS SES.

**Architecture:** A verification code (6 digits, 15-minute expiry) is generated and stored on the `users`
row at registration time, emailed via a new `EmailSender` port backed by an AWS SES adapter. Login checks
`email_verified` after the password check and rejects with a distinct `403` until the code is submitted
via a new `/verify-email` route. Mobile gains one new screen (code entry) inserted into the existing
registration flow.

**Tech Stack:** Go (backend, hexagonal/DDD, existing conventions), `github.com/aws/aws-sdk-go-v2/service/sesv2`
(new dependency), React Native / Expo (mobile), Jest + Go's `testing` package + this project's existing
`-tags integration` Postgres test convention.

**Spec:** `docs/superpowers/specs/2026-09-24-email-verification-design.md`

## Global Constraints

- AWS SES starts in **sandbox mode**: only pre-verified recipient addresses can receive anything until
  AWS approves a production-access request. This is an operational fact, not a bug to route around —
  Task 7 documents the manual console steps; nothing in the code tasks works around it.
- No deep-linking / clickable email links in v1 — `mobile/app.json` has no `scheme` configured, and
  adding one is out of scope. The code is typed into the app.
- `EmailContent`/`Message`/`Body`/`Content`/`Destination` field names and nesting below are copied
  verbatim from `go doc` run against the actually-installed `sesv2`/`sesv2/types` packages (verified
  during planning, not assumed from documentation summaries) — use them exactly as given, do not
  "correct" them from memory of an older or different AWS SDK shape.
- `VerifyEmail`/`ResendVerificationCode` take `email` as a raw `string` parameter, matching
  `LoginUser`'s existing convention (no `domain.NewEmail` re-validation at this layer) — do not add
  validation these sibling functions don't already have.
- The anti-enumeration stance already established by `ErrInvalidCredentials` (never reveals whether an
  email has an account) extends to `ResendVerificationCode`: it always returns `nil`, regardless of
  whether the email exists.

---

### Task 1: Domain & Ports — `User.emailVerified`, `EmailSender` port, `UserRepository` extension

**Files:**
- Modify: `backend/internal/domain/user.go`
- Create: `backend/internal/domain/user_test.go`
- Modify: `backend/internal/ports/user_repository.go`
- Create: `backend/internal/ports/email_sender.go`

**Interfaces:**
- Produces: `domain.User.EmailVerified() bool`; `domain.User.MarkEmailVerified() error`;
  `domain.ReconstructUser(id string, email Email, passwordHash PasswordHash, role Role, status UserStatus, emailVerified bool) *User`
  (signature changes — every existing call site must be updated in later tasks, there is exactly one
  today: `internal/adapters/postgres/user_repository.go`'s `scanUser`, updated in Task 2);
  `ports.EmailSender` interface; `ports.UserRepository.SaveVerificationCode`/`FindVerificationCode`
  method signatures (implemented in Task 2, consumed in Task 3).

- [ ] **Step 1: Write the failing domain test**

```go
// backend/internal/domain/user_test.go
package domain

import "testing"

func TestUser_NewUserStartsUnverified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)
	if u.EmailVerified() {
		t.Fatal("a newly registered user must start unverified")
	}
}

func TestUser_MarkEmailVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)

	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !u.EmailVerified() {
		t.Fatal("EmailVerified() should be true after MarkEmailVerified()")
	}
}

func TestUser_MarkEmailVerified_IdempotentOnAlreadyVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)

	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("second call should be a harmless no-op, got error: %v", err)
	}
	if !u.EmailVerified() {
		t.Fatal("still expected to be verified")
	}
}

func TestUser_MarkEmailVerified_RejectsDeletedUser(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)
	if err := u.Delete(); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	if err := u.MarkEmailVerified(); err != ErrUserDeleted {
		t.Fatalf("got error %v, want ErrUserDeleted", err)
	}
}

func TestReconstructUser_PreservesEmailVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")

	verified := ReconstructUser("id-1", email, hash, RoleUser, UserStatusActive, true)
	if !verified.EmailVerified() {
		t.Fatal("expected EmailVerified() true when reconstructed with true")
	}

	unverified := ReconstructUser("id-2", email, hash, RoleUser, UserStatusActive, false)
	if unverified.EmailVerified() {
		t.Fatal("expected EmailVerified() false when reconstructed with false")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd backend && go test ./internal/domain/... -run TestUser_ -v`
Expected: FAIL to compile — `EmailVerified`/`MarkEmailVerified` undefined, `ReconstructUser` called with
too many arguments.

- [ ] **Step 3: Modify `user.go`**

Add the field to the `User` struct:

```go
type User struct {
	id            string
	email         Email
	passwordHash  PasswordHash
	role          Role
	status        UserStatus
	emailVerified bool
}
```

Update `NewUser` to initialize it explicitly (new accounts always start unverified):

```go
func NewUser(email Email, passwordHash PasswordHash, role Role) *User {
	return &User{
		id:            newID(),
		email:         email,
		passwordHash:  passwordHash,
		role:          role,
		status:        UserStatusActive,
		emailVerified: false,
	}
}
```

Update `ReconstructUser`'s signature and body:

```go
// ReconstructUser rebâtit un User depuis des données déjà valides (une
// ligne Postgres) -- préserve l'ID, le statut, et l'état de vérification
// donnés, ne revalide rien.
func ReconstructUser(id string, email Email, passwordHash PasswordHash, role Role, status UserStatus, emailVerified bool) *User {
	return &User{
		id:            id,
		email:         email,
		passwordHash:  passwordHash,
		role:          role,
		status:        status,
		emailVerified: emailVerified,
	}
}
```

Add the new method, placed near `ChangeEmail`/`ChangePassword`/`ChangeRole` (the entity's existing
mutators):

```go
// MarkEmailVerified is idempotent -- re-submitting a still-valid code (or a
// stale client retrying) on an already-verified account is a harmless
// no-op, not an error. Only a deleted account is rejected, same guard every
// other mutator on this entity already uses.
func (u *User) MarkEmailVerified() error {
	if u.status == UserStatusDeleted {
		return ErrUserDeleted
	}
	u.emailVerified = true
	return nil
}
```

Add the new accessor next to the other `--- lecture ---` accessors at the bottom of the file:

```go
func (u *User) EmailVerified() bool { return u.emailVerified }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd backend && go test ./internal/domain/... -run TestUser_ -v` and
`go test ./internal/domain/... -run TestReconstructUser_PreservesEmailVerified -v`
Expected: PASS, all 5 tests. (The package won't fully build yet if any other file already calls
`ReconstructUser` with the old 5-argument signature — there is exactly one such call site,
`internal/adapters/postgres/user_repository.go`, fixed in Task 2. If `go build ./...` fails elsewhere in
the repo at this point because of that, that's expected and resolves in Task 2 — the domain package's
own tests still compile and pass in isolation.)

- [ ] **Step 5: Write the new ports**

Add two methods to the existing interface in `backend/internal/ports/user_repository.go`:

```go
package ports

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/domain"
)

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
}
```

Create `backend/internal/ports/email_sender.go`:

```go
package ports

import "context"

type EmailSender interface {
	SendVerificationCode(ctx context.Context, toEmail, code string) error
}
```

- [ ] **Step 6: Commit**

```bash
cd backend
git add internal/domain/user.go internal/domain/user_test.go internal/ports/user_repository.go internal/ports/email_sender.go
git commit -m "domain+ports: email verification (User.emailVerified, EmailSender port)"
```

---

### Task 2: Postgres adapter — schema + `UserRepository` implementation

**Files:**
- Modify: `backend/internal/adapters/postgres/schema.sql`
- Modify: `backend/internal/adapters/postgres/user_repository.go`
- Modify: `backend/internal/adapters/postgres/user_repository_test.go`

**Interfaces:**
- Consumes: Task 1's `domain.ReconstructUser(..., emailVerified bool)`, `ports.UserRepository`'s two new
  method signatures.
- Produces: `postgres.UserRepository.SaveVerificationCode`/`FindVerificationCode` (satisfying the port),
  used by Task 3's application-layer code.

- [ ] **Step 1: Modify `schema.sql`**

Add the three new columns to the existing `CREATE TABLE users` statement:

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
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

(This file is applied wholesale to a fresh database — it does not run as a migration against an
already-populated one. Task 7 covers applying the equivalent `ALTER TABLE` to the two databases that
already have real data: the local dev Postgres container and the deployed EC2 instance's Postgres.)

- [ ] **Step 2: Write the failing integration test**

Add to `backend/internal/adapters/postgres/user_repository_test.go` (this file already has the
`//go:build integration` tag and a `testPool(t)` helper — add these test functions alongside the
existing ones):

```go
func TestUserRepository_SaveAndFindVerificationCode(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verify-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	if err := repo.SaveVerificationCode(ctx, user.ID(), "123456", expiresAt); err != nil {
		t.Fatalf("save verification code: %v", err)
	}

	code, gotExpiresAt, err := repo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("got code %q, want %q", code, "123456")
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("got expiry %v, want %v", gotExpiresAt, expiresAt)
	}
}

func TestUserRepository_SaveVerificationCode_OverwritesPrevious(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verify-overwrite+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if err := repo.SaveVerificationCode(ctx, user.ID(), "111111", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save first code: %v", err)
	}
	if err := repo.SaveVerificationCode(ctx, user.ID(), "222222", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save second code: %v", err)
	}

	code, _, err := repo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if code != "222222" {
		t.Fatalf("got code %q, want the overwritten %q, not the first one", code, "222222")
	}
}

func TestUserRepository_FindVerificationCode_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("no-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, _, err := repo.FindVerificationCode(ctx, user.ID()); err != pgx.ErrNoRows {
		t.Fatalf("got error %v, want pgx.ErrNoRows for a user with no stored code", err)
	}
}

func TestUserRepository_EmailVerifiedRoundTrips(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verified-roundtrip+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	if err := user.MarkEmailVerified(); err != nil {
		t.Fatalf("mark verified: %v", err)
	}
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save verified user: %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if !found.EmailVerified() {
		t.Fatal("expected EmailVerified() true after reload from Postgres")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd backend && docker compose up -d postgres-test && go test -tags integration ./internal/adapters/postgres/... -run "TestUserRepository_.*VerificationCode|TestUserRepository_EmailVerifiedRoundTrips" -v`
Expected: FAIL to compile — `SaveVerificationCode`/`FindVerificationCode` undefined on `*UserRepository`,
`scanUser` doesn't select/scan the new column yet, `Save`'s call to `ReconstructUser`-shaped data is
still on the old 5-column INSERT.

- [ ] **Step 4: Modify `user_repository.go`**

Replace the whole file:

```go
package postgres

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

const upsertUserSQL = `
	INSERT INTO users (id, email, password_hash, role, status, email_verified)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (id) DO UPDATE SET
		email = EXCLUDED.email,
		password_hash = EXCLUDED.password_hash,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		email_verified = EXCLUDED.email_verified,
		updated_at = now()
`

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	_, err := r.db.Exec(ctx, upsertUserSQL,
		user.ID(), user.Email().String(), user.PasswordHash().String(), string(user.Role()), string(user.Status()), user.EmailVerified())
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status, email_verified FROM users WHERE id = $1
	`, id)
	return scanUser(row)
}

// FindByEmail sert au login : on cherche un compte par email, pas par ID.
// Contrairement à places.name, users.email a une contrainte UNIQUE (voir
// schema.sql) -- pas d'ambiguïté possible entre deux comptes de même email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status, email_verified FROM users WHERE email = $1
	`, email)
	return scanUser(row)
}

// SaveVerificationCode overwrites any previously stored code for this user
// -- a resend replaces the row's code/expiry in place, it never keeps a
// history of past codes.
func (r *UserRepository) SaveVerificationCode(ctx context.Context, userID, code string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET verification_code = $1, verification_code_expires_at = $2, updated_at = now() WHERE id = $3
	`, code, expiresAt, userID)
	return err
}

// FindVerificationCode returns pgx.ErrNoRows (the same sentinel
// FindByID/FindByEmail already surface unwrapped) when the user has no
// currently-stored code -- never registered one, or it was already cleared
// after a successful verification.
func (r *UserRepository) FindVerificationCode(ctx context.Context, userID string) (string, time.Time, error) {
	var code *string
	var expiresAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT verification_code, verification_code_expires_at FROM users WHERE id = $1
	`, userID).Scan(&code, &expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	if code == nil || expiresAt == nil {
		return "", time.Time{}, pgx.ErrNoRows
	}
	return *code, *expiresAt, nil
}

func scanUser(row rowScanner) (*domain.User, error) {
	var id, emailRaw, passwordHashRaw, roleRaw, statusRaw string
	var emailVerified bool
	if err := row.Scan(&id, &emailRaw, &passwordHashRaw, &roleRaw, &statusRaw, &emailVerified); err != nil {
		return nil, err
	}

	email, err := domain.NewEmail(emailRaw)
	if err != nil {
		return nil, err
	}
	passwordHash, err := domain.NewPasswordHash(passwordHashRaw)
	if err != nil {
		return nil, err
	}
	role, err := domain.NewRole(roleRaw)
	if err != nil {
		return nil, err
	}

	return domain.ReconstructUser(id, email, passwordHash, role, domain.UserStatus(statusRaw), emailVerified), nil
}
```

Note the added `"github.com/jackc/pgx/v5"` import is needed for `pgx.ErrNoRows` in
`FindVerificationCode` — add it to the import block above `"rioaudioguide/backend/internal/domain"`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test -tags integration ./internal/adapters/postgres/... -run "TestUserRepository_" -v`
Expected: PASS, all `TestUserRepository_*` tests (the pre-existing ones plus the 4 new ones from Step 2).

- [ ] **Step 6: Apply the schema change to the local dev database**

The local dev Postgres container already has data (from earlier sessions) and won't pick up
`schema.sql`'s new columns automatically — apply the equivalent `ALTER TABLE` directly:

```bash
docker exec backend-postgres-1 psql -U postgres -d postgres -c "
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS verification_code TEXT,
  ADD COLUMN IF NOT EXISTS verification_code_expires_at TIMESTAMPTZ;
"
```

Expected: `ALTER TABLE` (no error). Existing rows get `email_verified = false` by the column default —
acceptable per the spec's explicit note that pre-existing test accounts re-verify once, same as any new
account.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/postgres/schema.sql internal/adapters/postgres/user_repository.go internal/adapters/postgres/user_repository_test.go
git commit -m "postgres: email_verified + verification code columns, repository methods"
```

---

### Task 3: Application layer — registration/verification/resend/login logic

**Files:**
- Modify: `backend/internal/application/register_user.go`
- Create: `backend/internal/application/verify_email.go`
- Create: `backend/internal/application/resend_verification_code.go`
- Modify: `backend/internal/application/login_user.go`
- Create: `backend/internal/application/register_user_test.go`
- Create: `backend/internal/application/verify_email_test.go`
- Modify: `backend/internal/application/login_user_test.go` (does not exist yet — created fresh by this
  task, alongside the new `LoginUser` test case)

**Interfaces:**
- Consumes: Task 1's `ports.EmailSender`, `ports.UserRepository`'s extended interface, `domain.User`'s
  `EmailVerified()`/`MarkEmailVerified()`.
- Produces: `application.RegisterUser(ctx, userRepo, emailSender, email, plaintextPassword string, role domain.Role) (*domain.User, error)`
  (signature changes — the one existing call site, `internal/adapters/http/user_handler.go`'s
  `registerUser`, is updated in Task 5); `application.VerifyEmail(ctx, userRepo, email, code string) error`;
  `application.ResendVerificationCode(ctx, userRepo, emailSender, email string) error`;
  `application.ErrVerificationCodeInvalid`; `application.ErrEmailNotVerified` (new sentinel, alongside
  the existing `ErrInvalidCredentials` in `login_user.go`).

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/application/register_user_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

type fakeUserRepo struct {
	users            map[string]*domain.User
	byEmail          map[string]string // email -> userID
	codes            map[string]string
	codeExpiries     map[string]time.Time
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		users:        map[string]*domain.User{},
		byEmail:      map[string]string{},
		codes:        map[string]string{},
		codeExpiries: map[string]time.Time{},
	}
}

func (f *fakeUserRepo) Save(_ context.Context, u *domain.User) error {
	f.users[u.ID()] = u
	f.byEmail[u.Email().String()] = u.ID()
	return nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (f *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.byEmail[email]
	if !ok {
		return nil, errors.New("user not found")
	}
	return f.users[id], nil
}

func (f *fakeUserRepo) SaveVerificationCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	f.codes[userID] = code
	f.codeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeUserRepo) FindVerificationCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.codes[userID]
	if !ok {
		return "", time.Time{}, errors.New("no code found")
	}
	return code, f.codeExpiries[userID], nil
}

type fakeEmailSender struct {
	sentTo   []string
	sentCode []string
}

func (f *fakeEmailSender) SendVerificationCode(_ context.Context, toEmail, code string) error {
	f.sentTo = append(f.sentTo, toEmail)
	f.sentCode = append(f.sentCode, code)
	return nil
}

type erroringEmailSender struct{}

func (erroringEmailSender) SendVerificationCode(context.Context, string, string) error {
	return errors.New("ses is down")
}

func TestRegisterUser_GeneratesAndSendsVerificationCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	user, err := RegisterUser(context.Background(), repo, sender, "new@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.EmailVerified() {
		t.Fatal("a freshly registered user must not be verified yet")
	}

	if len(sender.sentTo) != 1 || sender.sentTo[0] != "new@example.com" {
		t.Fatalf("expected exactly one email sent to new@example.com, got %v", sender.sentTo)
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

	_, err := RegisterUser(context.Background(), repo, erroringEmailSender{}, "fail@example.com", "password123", domain.RoleUser)
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

Create `backend/internal/application/verify_email_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestVerifyEmail_CorrectCodeMarksVerified(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "verify@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]

	if err := VerifyEmail(context.Background(), repo, "verify@example.com", code); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindByID(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if !found.EmailVerified() {
		t.Fatal("expected the account to be verified after a correct code")
	}
}

func TestVerifyEmail_WrongCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "wrong-code@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}

	err := VerifyEmail(context.Background(), repo, "wrong-code@example.com", "000000")
	if !errors.Is(err, ErrVerificationCodeInvalid) {
		t.Fatalf("got error %v, want ErrVerificationCodeInvalid", err)
	}
}

func TestVerifyEmail_ExpiredCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "expired@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]
	// Overwrite with an already-past expiry to simulate a stale code.
	if err := repo.SaveVerificationCode(context.Background(), user.ID(), code, time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("save expired code: %v", err)
	}

	err = VerifyEmail(context.Background(), repo, "expired@example.com", code)
	if !errors.Is(err, ErrVerificationCodeInvalid) {
		t.Fatalf("got error %v, want ErrVerificationCodeInvalid", err)
	}
}

func TestResendVerificationCode_GeneratesADifferentCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "resend@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	firstCode := sender.sentCode[0]

	if err := ResendVerificationCode(context.Background(), repo, sender, "resend@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sentCode) != 2 {
		t.Fatalf("expected 2 emails sent total, got %d", len(sender.sentCode))
	}
	secondCode := sender.sentCode[1]
	if secondCode == firstCode {
		t.Fatal("expected the resent code to differ from the original (astronomically unlikely to collide by chance for a 6-digit code across two independent draws)")
	}

	storedCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if storedCode != secondCode {
		t.Fatalf("stored code %q should match the resent code %q, not the original", storedCode, secondCode)
	}
}

func TestResendVerificationCode_UnknownEmailReturnsNilSilently(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	if err := ResendVerificationCode(context.Background(), repo, sender, "nobody@example.com"); err != nil {
		t.Fatalf("expected nil error for an unknown email (anti-enumeration), got %v", err)
	}
	if len(sender.sentTo) != 0 {
		t.Fatal("expected no email sent for an unknown address")
	}
}
```

Create `backend/internal/application/login_user_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

// fakeTokenIssuer must satisfy the full ports.TokenIssuer interface
// (Issue AND Verify) to type-check as the tokens parameter LoginUser
// expects -- Verify is stubbed since these tests never call it.
type fakeTokenIssuer struct{}

func (fakeTokenIssuer) Issue(userID string, role domain.Role) (string, error) {
	return "fake-token-" + userID, nil
}

func (fakeTokenIssuer) Verify(token string) (string, domain.Role, error) {
	return "", "", errors.New("not implemented in fake")
}

func TestLoginUser_RejectsUnverifiedAccount(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "unverified@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := LoginUser(context.Background(), repo, fakeTokenIssuer{}, "unverified@example.com", "password123")
	if !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("got error %v, want ErrEmailNotVerified", err)
	}
}

func TestLoginUser_SucceedsOnceVerified(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "will-verify@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]
	if err := VerifyEmail(context.Background(), repo, "will-verify@example.com", code); err != nil {
		t.Fatalf("verify: %v", err)
	}

	token, err := LoginUser(context.Background(), repo, fakeTokenIssuer{}, "will-verify@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/application/... -run "TestRegisterUser_|TestVerifyEmail_|TestResendVerificationCode_|TestLoginUser_" -v`
Expected: FAIL to compile — `RegisterUser` called with the wrong number of arguments, `VerifyEmail`/
`ResendVerificationCode`/`ErrVerificationCodeInvalid`/`ErrEmailNotVerified` undefined.

- [ ] **Step 3: Add the shared code-generation helper**

Create `backend/internal/application/verification_code.go`:

```go
package application

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// generateVerificationCode produces a random 6-digit numeric string
// (leading zeros preserved via %06d) -- crypto/rand, not math/rand, since
// this is a short-lived credential even though it's low-stakes (15-minute
// expiry, single use).
func generateVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
```

- [ ] **Step 4: Modify `register_user.go`**

Replace the whole file:

```go
package application

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// verificationCodeTTL is how long a freshly generated code stays valid --
// 15 minutes, per the design spec.
const verificationCodeTTL = 15 * time.Minute

// RegisterUser hashes the plaintext password with bcrypt -- the domain
// PasswordHash Value Object only validates "non-empty", it has no idea how
// a hash is produced. The plaintext password never leaves this function:
// it's hashed immediately and discarded.
//
// Email uniqueness isn't pre-checked with a FindByEmail round trip: the
// users.email UNIQUE constraint (schema.sql) is the actual source of truth,
// so a duplicate surfaces as a Save error instead of a racy check-then-act.
//
// After the account is persisted, a 6-digit verification code is generated,
// stored, and emailed. If the email send fails, the error is returned to
// the caller -- but the account and its stored code are NOT rolled back:
// a transient email-provider failure shouldn't destroy a just-created
// account, and the client can always fall back to the resend endpoint.
func RegisterUser(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, plaintextPassword string, role domain.Role) (*domain.User, error) {
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
	if err := emailSender.SendVerificationCode(ctx, user.Email().String(), code); err != nil {
		return nil, err
	}

	return user, nil
}
```

- [ ] **Step 5: Write `verify_email.go`**

```go
package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// ErrVerificationCodeInvalid covers a wrong code, an expired one, and "no
// code found at all" -- deliberately not distinguished, there's no
// operational reason to tell a client which of the three happened.
var ErrVerificationCodeInvalid = errors.New("application: invalid or expired verification code")

func VerifyEmail(ctx context.Context, userRepo ports.UserRepository, email, code string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrVerificationCodeInvalid
	}

	storedCode, expiresAt, err := userRepo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		return ErrVerificationCodeInvalid
	}

	// Constant-time compare: a short numeric code checked over the network
	// is worth the trivial cost to avoid a timing side-channel, even though
	// the practical risk here is low.
	if subtle.ConstantTimeCompare([]byte(storedCode), []byte(code)) != 1 {
		return ErrVerificationCodeInvalid
	}
	if time.Now().After(expiresAt) {
		return ErrVerificationCodeInvalid
	}

	if err := user.MarkEmailVerified(); err != nil {
		return err
	}
	if err := userRepo.Save(ctx, user); err != nil {
		return err
	}
	// Clear the code so it can't be replayed -- an already-past expiry
	// makes any future FindVerificationCode/compare fail regardless of
	// what value is stored, so overwriting with an expired marker is
	// sufficient without needing a separate "clear" repository method.
	return userRepo.SaveVerificationCode(ctx, user.ID(), "", time.Now().Add(-1*time.Hour))
}
```

- [ ] **Step 6: Write `resend_verification_code.go`**

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
func ResendVerificationCode(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email string) error {
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
	return emailSender.SendVerificationCode(ctx, user.Email().String(), code)
}
```

- [ ] **Step 7: Modify `login_user.go`**

```go
package application

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrInvalidCredentials is deliberately the SAME error whether the email
// doesn't exist or the password is wrong -- distinguishing the two in the
// response would let an attacker enumerate which emails have accounts.
var ErrInvalidCredentials = errors.New("application: invalid email or password")

// ErrEmailNotVerified is deliberately distinct from ErrInvalidCredentials:
// the caller has already proven they know the correct password for this
// exact email, so confirming "this account needs verification" leaks no
// information an attacker didn't already have -- unlike ErrInvalidCredentials,
// there is no enumeration risk here.
var ErrEmailNotVerified = errors.New("application: email not verified")

// LoginUser verifies the password against the stored bcrypt hash and, on
// success, issues a signed JWT carrying the user's ID and role -- nothing
// else goes in the token (see internal/adapters/jwt: the payload is
// readable by anyone, never a place for secrets).
func LoginUser(ctx context.Context, userRepo ports.UserRepository, tokens ports.TokenIssuer, email, plaintextPassword string) (string, error) {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	if user.Status() == domain.UserStatusDeleted {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash().String()), []byte(plaintextPassword)); err != nil {
		return "", ErrInvalidCredentials
	}

	if !user.EmailVerified() {
		return "", ErrEmailNotVerified
	}

	return tokens.Issue(user.ID(), user.Role())
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/application/... -run "TestRegisterUser_|TestVerifyEmail_|TestResendVerificationCode_|TestLoginUser_" -v`
Expected: PASS, all tests across the three new/modified files.

- [ ] **Step 9: Run the full application package test suite to confirm nothing else broke**

Run: `cd backend && go test ./internal/application/... -v`
Expected: PASS. If any pre-existing test in this package constructs a `fakeUserRepo`-shaped type of its
own for an unrelated test (unlikely, but check the `go test` output for compile errors naming a
duplicate type) — this task's `fakeUserRepo`/`fakeEmailSender`/`fakeTokenIssuer` are new types local to
this package's test files; if a name collision surfaces, rename this task's fakes with a
`verification`-prefixed name (e.g. `fakeVerificationUserRepo`) rather than touching unrelated existing
test code.

- [ ] **Step 10: Commit**

```bash
git add internal/application/register_user.go internal/application/verify_email.go internal/application/resend_verification_code.go internal/application/login_user.go internal/application/verification_code.go internal/application/register_user_test.go internal/application/verify_email_test.go internal/application/login_user_test.go
git commit -m "application: email verification (register/verify/resend/login)"
```

---

### Task 4: AWS SES adapter

**Files:**
- Create: `backend/internal/adapters/awsses/sender.go`
- Create: `backend/internal/adapters/awsses/sender_test.go`
- Modify: `backend/go.mod`, `backend/go.sum` (new dependency)

**Interfaces:**
- Consumes: Task 1's `ports.EmailSender`.
- Produces: `awsses.NewSender(client sesAPI, fromEmail string) *Sender` — constructed in Task 5's
  `main.go` wiring, satisfying `ports.EmailSender`.

- [ ] **Step 1: Add the dependency**

Run: `cd backend && go get github.com/aws/aws-sdk-go-v2/service/sesv2@latest`

- [ ] **Step 2: Write the failing test**

Create `backend/internal/adapters/awsses/sender_test.go`:

```go
package awsses

import (
	"context"
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

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "123456"); err != nil {
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

func TestSender_SendVerificationCode_IncludesCodeInBody(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := fake.lastInput.Content
	if content == nil || content.Simple == nil || content.Simple.Body == nil || content.Simple.Body.Text == nil || content.Simple.Body.Text.Data == nil {
		t.Fatalf("expected a plain-text body, got %+v", content)
	}
	body := *content.Simple.Body.Text.Data
	if !strings.Contains(body, "654321") {
		t.Fatalf("body %q does not contain the code %q", body, "654321")
	}
}

func TestSender_SendVerificationCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}
```

Add `"errors"` and `"strings"` to this test file's imports alongside `"context"` and `"testing"`.

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd backend && go test ./internal/adapters/awsses/... -v`
Expected: FAIL to compile — package `awsses` and `NewSender` don't exist yet.

- [ ] **Step 4: Write `sender.go`**

```go
// backend/internal/adapters/awsses/sender.go
package awsses

import (
	"context"
	"fmt"

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

func (s *Sender) SendVerificationCode(ctx context.Context, toEmail, code string) error {
	subject := "Your Memória Carioca verification code"
	body := fmt.Sprintf("Your verification code is %s. It expires in 15 minutes.", code)

	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &s.from,
		Destination: &types.Destination{
			ToAddresses: []string{toEmail},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: &subject},
				Body: &types.Body{
					Text: &types.Content{Data: &body},
				},
			},
		},
	})
	return err
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd backend && go test ./internal/adapters/awsses/... -v`
Expected: PASS, all 3 tests.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/awsses/
git commit -m "adapters: AWS SES email sender"
```

---

### Task 5: HTTP routes + `main.go` wiring

**Files:**
- Modify: `backend/internal/adapters/http/server.go`
- Modify: `backend/internal/adapters/http/user_handler.go`
- Create: `backend/internal/adapters/http/user_handler_test.go` (does not exist yet)
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: Task 3's `application.RegisterUser` (new signature), `VerifyEmail`, `ResendVerificationCode`,
  `ErrVerificationCodeInvalid`, `ErrEmailNotVerified`; Task 4's `awsses.NewSender`.
- Produces: `POST /verify-email`, `POST /resend-verification-code` routes; `Server`'s new
  `emailSender ports.EmailSender` field/constructor parameter (position: appended at the end of
  `NewServer`'s existing parameter list, to avoid reordering every other call site's positional
  arguments).

- [ ] **Step 1: Write the failing HTTP handler tests**

Create `backend/internal/adapters/http/user_handler_test.go`. This project's existing HTTP handler
tests aren't present for `user_handler.go` today (no prior file to match against) — this test uses the
same fake-repository-and-real-`echo.Echo`-instance pattern as this codebase's other adapter tests
(`internal/adapters/postgres`'s fakes, `internal/application`'s fakes): reuse Task 3's `fakeUserRepo`
shape isn't directly importable across packages (it's unexported, package-local to `application`), so
this test defines its own small equivalent fake, scoped to this file:

```go
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

type fakeHTTPUserRepo struct {
	users        map[string]*domain.User
	byEmail      map[string]string
	codes        map[string]string
	codeExpiries map[string]time.Time
}

func newFakeHTTPUserRepo() *fakeHTTPUserRepo {
	return &fakeHTTPUserRepo{
		users:        map[string]*domain.User{},
		byEmail:      map[string]string{},
		codes:        map[string]string{},
		codeExpiries: map[string]time.Time{},
	}
}

func (f *fakeHTTPUserRepo) Save(_ context.Context, u *domain.User) error {
	f.users[u.ID()] = u
	f.byEmail[u.Email().String()] = u.ID()
	return nil
}

func (f *fakeHTTPUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (f *fakeHTTPUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.byEmail[email]
	if !ok {
		return nil, errors.New("not found")
	}
	return f.users[id], nil
}

func (f *fakeHTTPUserRepo) SaveVerificationCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	f.codes[userID] = code
	f.codeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeHTTPUserRepo) FindVerificationCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.codes[userID]
	if !ok {
		return "", time.Time{}, errors.New("not found")
	}
	return code, f.codeExpiries[userID], nil
}

type fakeHTTPEmailSender struct {
	lastCode string
}

func (f *fakeHTTPEmailSender) SendVerificationCode(_ context.Context, _, code string) error {
	f.lastCode = code
	return nil
}

func newTestServerForUserHandlers(userRepo *fakeHTTPUserRepo, emailSender *fakeHTTPEmailSender) *Server {
	return NewServer(nil, nil, nil, userRepo, nil, nil, nil, nil, fakeTokenIssuerForHTTP{}, nil, nil, emailSender)
}

// Must satisfy the full ports.TokenIssuer interface (Issue AND Verify) to
// type-check as NewServer's tokens parameter -- Verify is stubbed since
// none of this file's tests exercise a route that calls it.
type fakeTokenIssuerForHTTP struct{}

func (fakeTokenIssuerForHTTP) Issue(userID string, role domain.Role) (string, error) {
	return "fake-token", nil
}

func (fakeTokenIssuerForHTTP) Verify(token string) (string, domain.Role, error) {
	return "", "", errors.New("not implemented in fake")
}

func TestVerifyEmailHandler_CorrectCodeReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "handler@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "handler@example.com", "code": emailSender.lastCode})
	req := httptest.NewRequest("POST", "/verify-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestVerifyEmailHandler_WrongCodeReturns422(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "handler2@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "handler2@example.com", "code": "000000"})
	req := httptest.NewRequest("POST", "/verify-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 422 {
		t.Fatalf("got status %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestResendVerificationCodeHandler_AlwaysReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "nobody@example.com"})
	req := httptest.NewRequest("POST", "/resend-verification-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200 even for an unknown email, body: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginHandler_ReturnsForbiddenWhenNotVerified(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "unverified-http@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "unverified-http@example.com", "password": "password123"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf("got status %d, want 403, body: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/http/... -run "TestVerifyEmailHandler_|TestResendVerificationCodeHandler_|TestLoginHandler_ReturnsForbiddenWhenNotVerified" -v`
Expected: FAIL to compile — `NewServer` called with 12 args against an 11-arg signature, `/verify-email`/
`/resend-verification-code` routes don't exist (404).

- [ ] **Step 3: Modify `server.go`**

Add the field, append the constructor parameter, wire it into the struct literal, and register the two
new routes plus the existing `/register`/`/login` lines (unchanged registration, just shown for
context):

```go
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	userRepo      ports.UserRepository
	itineraryRepo ports.ItineraryRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
	cache         ports.Cache
	tokens        ports.TokenIssuer
	generator     ports.ItineraryGenerator
	assistant     ports.PlaceAssistant
	emailSender   ports.EmailSender
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, userRepo ports.UserRepository, itineraryRepo ports.ItineraryRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache, tokens ports.TokenIssuer, generator ports.ItineraryGenerator, assistant ports.PlaceAssistant, emailSender ports.EmailSender) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		userRepo:      userRepo,
		itineraryRepo: itineraryRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
		tokens:        tokens,
		generator:     generator,
		assistant:     assistant,
		emailSender:   emailSender,
	}
```

In the routes section, add the two new routes right after the existing `/register`/`/login` lines:

```go
	s.echo.POST("/register", s.registerUser)
	s.echo.POST("/login", s.login)
	s.echo.POST("/verify-email", s.verifyEmail)
	s.echo.POST("/resend-verification-code", s.resendVerificationCode)
	s.echo.POST("/logout", s.logout, auth)
```

- [ ] **Step 4: Modify `user_handler.go`**

Update `registerUser` to pass `s.emailSender`, and add the two new handlers plus the `login` handler's
new `403` branch:

```go
func (s *Server) registerUser(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	user, err := application.RegisterUser(c.Request().Context(), s.userRepo, s.emailSender, req.Email, req.Password, domain.RoleUser)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, userResponse{ID: user.ID(), Email: user.Email().String(), Role: user.Role().String()})
}
```

Modify the `login` handler's error branch to add the new `403` case (insert this `if` before the
existing `errors.Is(err, application.ErrInvalidCredentials)` check, or after — order between the two
doesn't matter since they're mutually exclusive, but insert it cleanly rather than nesting):

```go
func (s *Server) login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	token, err := application.LoginUser(c.Request().Context(), s.userRepo, s.tokens, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, application.ErrEmailNotVerified) {
			return c.JSON(http.StatusForbidden, echo.Map{"error": "email not verified"})
		}
		// Toujours 401 générique ici, jamais de détail sur "email inconnu" vs
		// "mot de passe faux" -- ApplicationErrInvalidCredentials existe
		// justement pour ne jamais faire cette distinction.
		if errors.Is(err, application.ErrInvalidCredentials) {
			return c.JSON(http.StatusUnauthorized, echo.Map{"error": "invalid email or password"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, loginResponse{Token: token})
}
```

Add the two new handler functions and their request types, placed after `login`:

```go
type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (s *Server) verifyEmail(c echo.Context) error {
	var req verifyEmailRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if err := application.VerifyEmail(c.Request().Context(), s.userRepo, req.Email, req.Code); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": "invalid or expired code"})
	}
	return c.JSON(http.StatusOK, echo.Map{})
}

type resendVerificationCodeRequest struct {
	Email string `json:"email"`
}

// resendVerificationCode always returns 200, whether or not the email
// belongs to a real account -- application.ResendVerificationCode already
// enforces this anti-enumeration behavior, this handler just doesn't
// second-guess it.
func (s *Server) resendVerificationCode(c echo.Context) error {
	var req resendVerificationCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if err := application.ResendVerificationCode(c.Request().Context(), s.userRepo, s.emailSender, req.Email); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/adapters/http/... -v`
Expected: PASS, all tests in this package (the 4 new ones plus any pre-existing ones in this directory
continuing to pass).

- [ ] **Step 6: Modify `cmd/api/main.go`**

Add the SES client construction and pass it into `NewServer`. Insert this near the existing S3 client
construction (both use the same `awsCfg` already loaded):

```go
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audio-guide"))

	sesClient := sesv2.NewFromConfig(awsCfg)
	emailSender := awsses.NewSender(sesClient, envOr("SES_SENDER_EMAIL", "noreply@example.com"))
```

Add the two new imports to the import block:

```go
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
```
(alphabetically among the other `github.com/aws/aws-sdk-go-v2/service/...` imports), and:
```go
	"rioaudioguide/backend/internal/adapters/awsses"
```
(alphabetically among the other `rioaudioguide/backend/internal/adapters/...` imports).

Update the `NewServer` call to append `emailSender`:

```go
	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, userRepo, itineraryRepo, publisher, storage, cache, tokens, itineraryGenerator, placeAssistant, emailSender)
```

- [ ] **Step 7: Build the whole backend to confirm everything compiles together**

Run: `cd backend && go build ./...`
Expected: no output, exit 0. (`cmd/worker/main.go` doesn't construct a `Server` and doesn't need this
change — confirm it still builds as part of `./...` without modification.)

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/http/server.go internal/adapters/http/user_handler.go internal/adapters/http/user_handler_test.go cmd/api/main.go
git commit -m "http: /verify-email, /resend-verification-code routes; login gates on verification"
```

---

### Task 6: Mobile — registration flow, verification screen

**Files:**
- Modify: `mobile/src/data/AuthRepository.ts`
- Modify: `mobile/src/auth/AuthContext.tsx`
- Modify: `mobile/src/screens/Auth.tsx`
- Create: `mobile/src/screens/VerifyEmail.tsx`
- Modify: `mobile/src/navigation/types.ts`
- Modify: `mobile/src/navigation/AppNavigator.tsx`
- Modify: `mobile/src/i18n/dictionary.ts`

**Interfaces:**
- Consumes: Task 5's `POST /verify-email` (`{email, code}` → `200`/`422`), `POST /resend-verification-code`
  (`{email}` → always `200`), `POST /login`'s new `403` response.
- Produces: `AuthRepository.verifyEmail(email, code): Promise<void>`,
  `AuthRepository.resendVerificationCode(email): Promise<void>`; `AppStackParamList`'s new
  `VerifyEmail: { email: string; password: string }` route.

- [ ] **Step 1: Add the two new `AuthRepository.ts` functions**

Add after the existing `deleteAccount` function:

```ts
export async function verifyEmail(email: string, code: string): Promise<void> {
  await authFetch<Record<string, never>>("/verify-email", { method: "POST", body: { email, code } });
}

export async function resendVerificationCode(email: string): Promise<void> {
  await authFetch<Record<string, never>>("/resend-verification-code", { method: "POST", body: { email } });
}
```

(`authFetch` already throws `AuthApiError` on a non-2xx response — a `422` from `/verify-email` surfaces
the same way `login`'s existing `401` already does, no new error-handling plumbing needed here.)

- [ ] **Step 2: Modify `AuthContext.tsx`**

Change `registerFn` to stop auto-logging in (login would now fail with `403` on a freshly-registered,
unverified account anyway):

```ts
  async function registerFn(email: string, password: string) {
    await Auth.register(email, password);
    // Deliberately no auto-login here anymore: a freshly registered
    // account isn't verified yet, and /login now rejects with 403 until it
    // is. AuthScreen.submit() navigates to VerifyEmail next, which calls
    // login() itself once the code is confirmed.
  }
```

Add `verifyEmail` and `resendVerificationCode` to `AuthContextValue`'s type and to the `useMemo` value
object, both simply forwarding to `Auth.*` (no local state to manage — these don't change `token`/`user`):

```ts
type AuthContextValue = {
  user: AuthUser | null;
  token: string | null;
  isLoggedIn: boolean;
  isLoading: boolean;
  register: (email: string, password: string) => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  updateProfile: (changes: { email?: string; password?: string }) => Promise<void>;
  deleteAccount: () => Promise<void>;
  verifyEmail: (email: string, code: string) => Promise<void>;
  resendVerificationCode: (email: string) => Promise<void>;
};
```

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
    }),
    [user, token, isLoading],
  );
```

- [ ] **Step 3: Modify `Auth.tsx`**

Change `submit()` to route to the new screen on register, and to detect the new `403`-not-verified case
on login:

```tsx
  const { login, register } = useAuth();
```
becomes
```tsx
  const { login, register } = useAuth();
```
(unchanged — `verifyEmail`/`resendVerificationCode` are used by the new screen, not this one).

Replace `submit()`:

```tsx
  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      if (mode === "login") {
        await login(email.trim(), password);
        navigation.goBack();
      } else {
        await register(email.trim(), password);
        navigation.navigate("VerifyEmail", { email: email.trim(), password });
      }
    } catch (err) {
      if (err instanceof AuthApiError && err.status === 403) {
        // Registered previously but never verified (e.g. closed the app
        // before entering the code) -- route to the same screen rather
        // than showing a generic login error.
        navigation.navigate("VerifyEmail", { email: email.trim(), password });
      } else {
        setError(err instanceof AuthApiError ? t.auth.genericError : t.auth.networkError);
      }
    } finally {
      setSubmitting(false);
    }
  }
```

- [ ] **Step 4: Add the new dictionary keys (all 4 locales)**

In `mobile/src/i18n/dictionary.ts`, each of the 4 `auth: { ... }` blocks (search for `auth: {` — there
are 4 occurrences, one per locale, at approximately lines 112, 279, 446, 613 as of this plan's writing)
gains these keys, added right after the existing `networkError` line in each block:

- `fr`:
  ```
  emailNotVerified: "Vérifie ton e-mail avant de te connecter.",
  ```
- `en`:
  ```
  emailNotVerified: "Verify your email before logging in.",
  ```
- `pt`:
  ```
  emailNotVerified: "Verifique seu e-mail antes de fazer login.",
  ```
- `es`:
  ```
  emailNotVerified: "Verifica tu correo antes de iniciar sesión.",
  ```

(This key isn't actually shown anywhere by Step 3's code above — the `403` case silently redirects
instead of displaying an error. It's added here anyway because it documents the case for a future screen
that might want to explain the redirect, and costs nothing to have; if a reviewer considers an unused
string a real defect for this task, wiring a brief toast/subtitle on `VerifyEmail` using it — "you're
here because your email needs verifying" — is a reasonable one-line addition, implementer's call.)

Also add a new top-level `verifyEmail: { ... }` section to each of the 4 locale blocks (placed next to
the existing `auth: { ... }` section, same nesting level — not inside it):

- `fr`:
  ```ts
  verifyEmail: {
    title: "Vérifie ton e-mail",
    subtitle: "On t'a envoyé un code à 6 chiffres à {email}. Entre-le ci-dessous.",
    codePlaceholder: "123456",
    submitCta: "Valider",
    resendLink: "Renvoyer le code",
    resendSent: "Code renvoyé.",
    invalidCode: "Code invalide ou expiré.",
  },
  ```
- `en`:
  ```ts
  verifyEmail: {
    title: "Verify your email",
    subtitle: "We sent a 6-digit code to {email}. Enter it below.",
    codePlaceholder: "123456",
    submitCta: "Verify",
    resendLink: "Resend code",
    resendSent: "Code resent.",
    invalidCode: "Invalid or expired code.",
  },
  ```
- `pt`:
  ```ts
  verifyEmail: {
    title: "Verifique seu e-mail",
    subtitle: "Enviamos um código de 6 dígitos para {email}. Digite-o abaixo.",
    codePlaceholder: "123456",
    submitCta: "Verificar",
    resendLink: "Reenviar código",
    resendSent: "Código reenviado.",
    invalidCode: "Código inválido ou expirado.",
  },
  ```
- `es`:
  ```ts
  verifyEmail: {
    title: "Verifica tu correo",
    subtitle: "Te enviamos un código de 6 dígitos a {email}. Ingrésalo abajo.",
    codePlaceholder: "123456",
    submitCta: "Verificar",
    resendLink: "Reenviar código",
    resendSent: "Código reenviado.",
    invalidCode: "Código inválido o expirado.",
  },
  ```

- [ ] **Step 5: Add the new route to `navigation/types.ts`**

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
  VerifyEmail: { email: string; password: string };
};
```

- [ ] **Step 6: Write `VerifyEmail.tsx`**

Follow this app's existing screen conventions (see `mobile/src/screens/Auth.tsx` for the established
`SafeAreaView`/`useState`/`ActivityIndicator` shape and `mobile/src/theme/tokens.ts` for
`colors`/`fonts`/`spacing`/`radii`):

```tsx
// mobile/src/screens/VerifyEmail.tsx
import React, { useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { AuthApiError } from "../data/AuthRepository";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "VerifyEmail">;

export function VerifyEmailScreen({ route, navigation }: Props) {
  const { email, password } = route.params;
  const { t } = useLocale();
  const { verifyEmail, resendVerificationCode, login } = useAuth();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendMessage, setResendMessage] = useState<string | null>(null);

  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      await verifyEmail(email, code.trim());
      // Verification succeeded -- complete the login the user originally
      // started with (password was carried through via route params
      // specifically so they don't have to type it twice).
      await login(email, password);
      navigation.getParent()?.goBack();
    } catch (err) {
      setError(err instanceof AuthApiError ? t.verifyEmail.invalidCode : t.auth.networkError);
    } finally {
      setSubmitting(false);
    }
  }

  async function resend() {
    setResendMessage(null);
    setResending(true);
    try {
      await resendVerificationCode(email);
      setResendMessage(t.verifyEmail.resendSent);
    } catch {
      setError(t.auth.networkError);
    } finally {
      setResending(false);
    }
  }

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.form}>
        <Text style={styles.title}>{t.verifyEmail.title}</Text>
        <Text style={styles.subtitle}>{t.verifyEmail.subtitle.replace("{email}", email)}</Text>

        <TextInput
          style={styles.input}
          value={code}
          onChangeText={setCode}
          keyboardType="number-pad"
          maxLength={6}
          placeholder={t.verifyEmail.codePlaceholder}
          placeholderTextColor={colors.inkFaint}
        />

        {error ? <Text style={styles.error}>{error}</Text> : null}
        {resendMessage ? <Text style={styles.resendMessage}>{resendMessage}</Text> : null}

        <Pressable
          style={[styles.btn, (!code.trim() || submitting) && styles.btnDisabled]}
          onPress={submit}
          disabled={!code.trim() || submitting}
        >
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.verifyEmail.submitCta}</Text>}
        </Pressable>

        <Pressable style={styles.resendLinkWrap} onPress={resend} disabled={resending}>
          <Text style={styles.resendLinkText}>{t.verifyEmail.resendLink}</Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  form: { marginHorizontal: spacing.xl, marginTop: spacing.xl },
  title: { fontFamily: fonts.display, fontSize: 26, color: colors.ink, marginBottom: 10 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, marginBottom: spacing.lg },
  input: {
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

- [ ] **Step 7: Register the new screen in `AppNavigator.tsx`**

```tsx
import { VerifyEmailScreen } from "../screens/VerifyEmail";
```
(added to the import block, alongside the other screen imports)

```tsx
      <Stack.Screen name="Auth" component={AuthScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="VerifyEmail" component={VerifyEmailScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="EditProfile" component={EditProfileScreen} />
```
(added right after the existing `Auth` screen registration, same `presentation: "modal"` option — it's
reached only from within that same modal flow)

- [ ] **Step 8: Typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 9: Manual verification**

With the backend running locally and Task 5's routes live: register a new test account through the app,
confirm it lands on the new "Verify your email" screen instead of logging straight in. Since a real
local backend has no real SES access configured yet (Task 7 covers that), the actual email won't arrive
in this manual check — instead, read the code directly out of the local Postgres `users` table's
`verification_code` column (`docker exec backend-postgres-1 psql -U postgres -d postgres -c "SELECT
verification_code FROM users WHERE email = '<the test email>';"`) and type it into the app. Confirm
successful verification logs the user in and returns to the app's main screen.

- [ ] **Step 10: Commit**

```bash
git add src/data/AuthRepository.ts src/auth/AuthContext.tsx src/screens/Auth.tsx src/screens/VerifyEmail.tsx src/navigation/types.ts src/navigation/AppNavigator.tsx src/i18n/dictionary.ts
git commit -m "mobile: email verification screen, registration no longer auto-logs in"
```

---

### Task 7: AWS SES setup (manual, founder's own work) + deployed-database schema

**Files:** none (this task is a checklist to execute, plus one command against the already-deployed EC2
database — no repository files change).

**Interfaces:** none — this task has no code interface, it makes Tasks 1-6's code actually work against
the real AWS account and the already-running EC2 deployment.

- [ ] **Step 1: Verify a sender email identity in SES**

AWS Console → Simple Email Service → **Identities** → **Create identity** → choose **Email address** →
enter an address you own (your own address is fine for now — e.g. the same one used for the app's admin
account). SES emails a confirmation link to that address — click it. Takes a couple of minutes. This
becomes the value for `SES_SENDER_EMAIL`.

- [ ] **Step 2: Verify each recipient address you'll test with**

Still in **Identities** → **Create identity** → **Email address**, repeat for every email address you
intend to register test accounts with while SES is in sandbox mode (including whatever address you use
to test the flow yourself). Each one needs its own confirmation-link click.

- [ ] **Step 3: Grant SES permission to `rio-cicd`**

AWS Console → IAM → **Users** → `rio-cicd` → **Permissions** tab → **Add permissions** → **Attach
policies directly** → search `AmazonSESFullAccess` → check it → **Next** → **Add permissions**. (A more
tightly-scoped inline policy restricted to `ses:SendEmail`/`ses:SendRawEmail` on the specific verified
identity's ARN is preferable to match `rio-cicd`'s existing minimal-permissions posture, but the managed
policy is faster to attach today — tightening it later is a config-only change, not a code change.)

- [ ] **Step 4: Apply the schema change to the deployed database**

Same `ALTER TABLE` as Task 2 Step 6, run against the EC2 instance's Postgres container instead of the
local one (via the AWS Console's EC2 Instance Connect, same as every other manual step this session
used). **This must run BEFORE Step 5's redeploy** — the new binary's `SELECT`/`INSERT` statements name
`email_verified` explicitly, so redeploying first would make every user-touching route (`/login`,
`/register`, `/me`, everything behind `requireAuth`) 500 until this runs. Running it first is safe
against the currently-deployed OLD binary too: it lists every column explicitly, so three new columns
with defaults are invisible to it.

```bash
sudo docker exec rio-backend-postgres-1 psql -U postgres -d postgres -c "
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS verification_code TEXT,
  ADD COLUMN IF NOT EXISTS verification_code_expires_at TIMESTAMPTZ;
"
```

- [ ] **Step 5: Set `SES_SENDER_EMAIL` on the deployed backend and redeploy**

From the same EC2 Instance Connect terminal session as Step 4:

```bash
cd ~/rio-backend
echo 'SES_SENDER_EMAIL=<the address verified in Step 1>' >> .env
sudo docker compose -f docker-compose.prod.yml up -d --build api
```

(`--build` picks up this plan's new code once it's been deployed to that instance via whatever
deployment step follows this plan's implementation — if the instance is still running an older image
without Tasks 1-6's changes, this command alone won't yet reflect them; that redeploy is outside this
plan's scope, the same way the rest of this session's live-testing loop has redeployed the backend
manually each time.)

- [ ] **Step 6: Request production access (not blocking, do when convenient)**

AWS Console → SES → **Account dashboard** → **Request production access**. Fill in the use case (a
personal/early-stage app sending account-verification emails). Not required for continued testing with
pre-verified addresses from Steps 1-2 — this only lifts the sandbox restriction for real, unverified
future users. No code depends on this step; it's here so it isn't forgotten once real users are closer.

## Self-review notes

- **Spec coverage:** every section of `docs/superpowers/specs/2026-09-24-email-verification-design.md`
  maps to a task above — Data model → Task 2; Domain → Task 1; Ports → Task 1; Application → Task 3;
  Adapter → Task 4; HTTP → Task 5; Mobile → Task 6; AWS setup → Task 7. Testing section's items are
  distributed across each task's own test steps, not a separate task.
- **Type consistency:** `ReconstructUser`'s new 6th parameter (`emailVerified bool`) is defined in Task 1
  and its one call site (`scanUser`) is updated in the same commit as the column it reads from (Task 2)
  — no task in between leaves the codebase in a non-compiling state for longer than its own steps.
  `RegisterUser`'s new `emailSender ports.EmailSender` parameter (Task 3) and its one call site
  (`registerUser` HTTP handler) are both updated together across Tasks 3 and 5 — Task 3's own tests
  construct `RegisterUser` with the new signature directly, so Task 3 is independently compilable and
  testable before Task 5 touches the handler.
- **No placeholders:** every step above contains complete, copy-pasteable code — no "add error handling"
  or "write tests for the above" left unfilled.
