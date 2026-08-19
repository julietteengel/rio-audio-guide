# Mobile app — user accounts (login, edit profile, logout, delete)

**Date:** 2026-08-19
**Branch/worktree:** `frontend`, `.worktrees/frontend/mobile/`
**Status:** done. Backend built in parallel by the founder on `backend` (commits `93a1af0` User
entity, `bbae160` JWT auth routes) — this spec covers the mobile side only; the backend's own auth
design is that branch's to document, not duplicated here.

## Why

Requested to support a bigger goal already underway on `backend`: syncing downloaded places/history/
favorites across a user's devices, which needs an account to attach that data to. This spec covers only
what exists today — login, profile edit, logout, delete — not the sync feature itself, which depends on
backend work not built yet.

## Decisions

- **Optional, not required.** The app's whole onboarding/positioning (already built) assumes zero
  friction to start listening — no screen anywhere gates on being logged in. An account is a layer on
  top, entered from Settings, never a wall in front of the core experience.
- **Email + password only, no Apple/Google.** The backend has no OAuth wired up (no provider/
  `apple_sub`/`google_sub` field on `User` — checked in `internal/domain/user.go` before assuming
  otherwise). Building login buttons for providers the backend can't handle would be dead UI. Apple
  Sign-In becomes a real requirement (App Store guideline 4.8) the moment Google login is added — worth
  building both together later, not Google alone.
- **Language stays local/per-device for now.** `User` has no language field on the backend today, so
  there is nothing to sync — Settings' language picker is unchanged. Revisit once the backend adds that
  field; this is a known gap, not an oversight.
- **Session read optimistically at launch, verified only reactively.** The JWT is stateless (24h TTL,
  `internal/adapters/jwt/issuer.go`) — there is nothing to check with the server that works offline
  anyway. The app reads whatever token/profile was last stored (`expo-secure-store`) and shows a
  logged-in UI immediately, no network round-trip at startup. An expired or revoked token is only
  discovered when an authenticated call (`updateProfile`/`deleteAccount`) comes back 401, at which point
  the local session is cleared and the UI falls back to logged out. This answers "what happens if the
  app is offline at launch" without any special-case code — it falls out of the stateless-JWT design.
- **Delete is destructive and confirmed.** A native `Alert.alert` with a destructive-styled confirm
  button gates `DELETE /me` — no silent or single-tap deletion. The confirmation copy states that
  downloads already on the device are unaffected (only server-side profile/sync data is removed) —
  worth re-checking once real cross-device sync exists, in case that changes.
- **No `/me` GET route exists** (only PATCH/DELETE) — login only returns a token, not a profile, so the
  profile shown right after login is built from what the user just typed rather than a server
  round-trip. Fine today (email is the only field); would need a real `/me` GET if the profile ever
  grows fields the client doesn't already know.

## What was built

`src/data/AuthRepository.ts` (register/login/logout/updateProfile/deleteAccount against the real
routes) → `src/auth/AuthContext.tsx` (session state, `expo-secure-store` persistence, the offline-
optimistic-read/reactive-401-logout behavior above) → `src/screens/Auth.tsx` (single screen, toggles
login/register mode) and `src/screens/EditProfile.tsx` → an "Account" section added at the top of
`Settings.tsx` (above "Données hors ligne"), matching the iOS convention of surfacing account identity
first in a settings screen.

## Verification

`npx tsc --noEmit` clean, `npx jest` 12/12 (unchanged — no new unit tests were added for this feature;
the UI is straightforward form/session-state code without the branching complexity that made the
offline-download logic worth testing separately). `npx expo export --platform ios` bundled clean
(1040 modules).

## Still open

- Real cross-device sync of downloaded places/history/favorites — the actual reason this was requested,
  not yet built (depends on further backend work).
- Language field on `User` / syncing the language preference through an account.
- A real `GET /me` if the profile grows beyond email.
- Password reset ("forgot password") flow — not built on either side yet.
