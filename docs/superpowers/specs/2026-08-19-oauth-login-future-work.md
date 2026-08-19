# OAuth login (Google/Apple) — deferred future work

Status: **not started, deliberately deferred**. Logged here so a future session picks this up without
re-deriving the analysis.

## Why this exists

The `frontend` branch's account screens (see `2026-08-19-mobile-account-design.md`) ship email+password
only for now — no Google/Apple sign-in buttons, because the backend has nothing to call yet. This doc
captures what OAuth login would need on the backend side, decided but not built.

## Mechanism

Mobile app uses the native Google Sign-In / Apple Sign-In SDK, gets back a provider-signed ID token
(a JWT), sends it to the backend. Backend verifies the signature against the provider's JWKS endpoint
(Google: `https://www.googleapis.com/oauth2/v3/certs`, Apple: `https://appleid.apple.com/auth/keys`),
checks `iss`/`aud`/`exp`, extracts the verified email, finds-or-creates a `User` by that email, issues
our own JWT exactly as the email/password flow already does. Everything downstream of that JWT issuance
is unchanged.

## What's blocking a start

1. **A real domain decision, not yet made**: `domain.User.passwordHash` is currently required
   (`NewPasswordHash` rejects empty). An OAuth-created account never sets a password. Needs either
   PasswordHash becoming optional on `User` (`HasPassword() bool`, login logic branches), or a separate
   auth-method concept. This is `internal/domain` — founder territory, needs the same kind of explicit
   scoped decision the `User` entity itself got.
2. **Real external registration, not a code task**: testing this for real needs a real Google OAuth
   Client ID (Google Cloud Console) and a real Apple Service ID (Apple Developer, "Sign in with Apple"
   capability) — account/business setup, not something written in this repo.

## What's NOT blocked (normal scope, once the domain decision above is made)

- `ports.OAuthVerifier` (or provider-specific ports) + adapters verifying Google/Apple ID tokens
- `application.LoginWithOAuth` use case (verify token, find-or-create user by email, issue JWT)
- `POST /login/google`, `POST /login/apple` routes

## Simplification adopted (revisit if it stops being true)

Matching accounts by verified email only, no separate `provider` + `provider_user_id` linked-identity
table. Fine for this project's current scope (small user base, one identity per email); would need
revisiting if a user should ever link multiple providers to one account, or if a provider's verified
email can't be trusted as the sole matching key.
