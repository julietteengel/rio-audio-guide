# First real ElevenLabs generation run — findings, fixes, error-handling audit

Status: **done for today**. Cristo Redentor is 3/4 languages generated for real; the other 14
places in the validated top-15 list are blocked on ElevenLabs quota, not on anything technical.
Logged here so a future session doesn't re-derive the same diagnosis.

## What we set out to do

Trigger real ElevenLabs generation (`POST /scripts/:id/review`) for Cristo Redentor's 4 languages
(FR/EN/ES/PT), as a first real test before doing the same for the other 14 places in the validated
top-15 touristic list (see the narrations review artifact from earlier in this session). Voices
were hand-picked per language by the founder, not auto-selected.

## What we built to get there (all real, tested, pushed)

- **`ScriptRepository.FindByPlaceID`** (`internal/ports`, explicit founder go-ahead) +
  **`application.MissingLanguages`**: given a place, which of its 4 languages have no *published*
  script yet (a script that exists but is still draft/reviewed — audio requested, not confirmed
  ready — counts as missing, same rule `GetPlaceAudio` already applied). Commit `cd51fd9`.
- **`requireRole(RoleAdmin)`** (`internal/adapters/http`): `POST /scripts/:id/review` was gated
  behind `requireAuth` only, meaning any authenticated `RoleUser` account could trigger a real,
  billed ElevenLabs call. Added a role-check middleware, restricted the route to `RoleAdmin`,
  bootstrapped the founder's own account to admin directly in Postgres (no self-service
  "become admin" route — that would defeat the point). Commit `6bec368`.
- **`POST /audio-files/:id/retry`**: re-queues a `failed` `AudioFile` with the voice_id it already
  had (`AudioFile.Retry()`, domain-guarded, already existed — no domain/ports change needed here).
  Built because we immediately needed it for real, not speculatively. Commit `c884925`.

## The real bug: unbounded retry loop on transient TTS errors

First 4 review calls all failed identically:
```
elevenlabs: request failed: ... context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

Root cause, confirmed against real data: Cristo Redentor's narrations are ~1,200–1,350 words each
(~7,300–7,900 characters). ElevenLabs' text-to-speech endpoint is **not streamed** — the response
only arrives once the *entire* audio file is synthesized server-side, and that time scales with
text length. The `Generator`'s HTTP client had a fixed `60 * time.Second` timeout, sized for short
text, never revisited once narrations got long.

The worse part wasn't the timeout itself — it's that `worker.go`'s transient-error branch was
`time.Sleep(requeueDelay); msg.Nack(false, true)`, **with no attempt cap**. RabbitMQ redelivers a
Nack'd-with-requeue message indefinitely. A persistently-too-slow (not broken, just slow) request
would retry forever, **silently re-calling and re-billing ElevenLabs every ~62 seconds**, never
surfacing as a real, actionable failure.

Fixed (commit `4a56cbd`):
- `Generator`'s timeout: `60s → 5 minutes`.
- Added `Attempt int` to `ttsJobMessage` (the RabbitMQ message body) and `maxTTSAttempts = 3`. A
  transient error now: Acks the current message, republishes a copy with `Attempt+1` via a new
  `requeueWithAttempt` (can't just Nack — Nack redelivers the *same* message unchanged, no way to
  carry a counter on it). Once `Attempt+1 >= maxTTSAttempts`, gives up cleanly: marks the
  `AudioFile` `failed` (visible, retriable via the new retry endpoint), Acks, stops.
- Real test proof: `TestWorker_TransientTTSError_GivesUpAfterMaxAttempts` — a generator that always
  fails is called **exactly** `maxTTSAttempts` times, then the `AudioFile` reaches `failed` with a
  real failure reason. Not just "eventually stops" — bounded and verified.

## Secondary finding from the same audit: silently discarded Ack/Nack errors

Every `msg.Ack()`/`msg.Nack()` call discarded its own return value (`_ = msg.Ack(false)`). Usually
harmless — a failed Ack/Nack typically means the connection is already dead, and RabbitMQ
redelivers to another consumer regardless of what our code does locally — but repeated failures
here are a real signal of connection instability worth surfacing, not absorbing silently. Added
`ackOrLog`/`nackOrLog` helpers, replaced all 10 call sites. Commit `da5692b`.

## Go error-handling audit (full pass, not just worker.go)

Asked to verify the codebase follows Go error-handling best practices. Checked systematically
rather than from memory:

**Already solid, confirmed by grep, not assumed:**
- Zero fully-swallowed errors anywhere in `internal/`/`cmd/` (every checked `err` either returns,
  logs, or is a deliberately-ignored cleanup call like `rows.Close()` — the one universally-accepted
  exception to "always check errors").
- `errors.Is` (sentinel matching, e.g. `pgx.ErrNoRows`) and `errors.As` (type matching, e.g.
  `*ports.PermanentError`) used correctly across 7 files — no fragile `err.Error() == "..."` string
  comparisons anywhere.
- `fmt.Errorf` wrapping: zero real violations. The two `fmt.Errorf` calls without `%w` both
  construct a *new* error from scratch (no underlying error exists to wrap), so `%w` doesn't apply
  — not a gap.
- `log.Fatal(f)` is confined entirely to `cmd/*/main()` — never inside `internal/`, which is exactly
  right: fatal-and-exit belongs at the true top level, never buried in business logic where it would
  block proper error propagation and testability.
- `ports.PermanentError` — a real, purpose-built type distinguishing "give up" from "retry",
  shared between the S3 and ElevenLabs adapters. Non-trivial design choice, not boilerplate.

**Gaps found and fixed, both above**: the unbounded-retry loop, and the discarded Ack/Nack errors.

## Real generation result: Cristo Redentor, 3/4

| Language | Result |
|---|---|
| FR | `ready` — `s3://rio-audio-guide/728b089d-....mp3`, ~10m54s |
| EN | `ready` — `s3://rio-audio-guide/4c93e68e-....mp3`, ~9m43s |
| ES | `ready` — `s3://rio-audio-guide/8e6cf996-....mp3`, ~10m46s |
| PT | `failed` — `quota_exceeded`: plan quota is 40000, had 1250 remaining, request needed 3748 |

The PT failure is **not a bug** — it's a real `PermanentError` (401, `quota_exceeded`), correctly
not retried (retrying an exhausted quota changes nothing), correctly surfaced as a clean, visible
failure instead of looping. The error-handling design held up exactly as intended on a real,
unplanned failure mode it wasn't specifically built for.

## The real constraint: quota, not code

3 of 4 languages for **one** place consumed nearly the entire 40,000-credit ElevenLabs quota. The
validated top-15 list is 15 places × 4 languages ≈ 56 more narrations of similar length — not
affordable on the current plan, and probably not on most plans, without changing something.

Recommendation given to the founder (her call, not decided here):
1. **Narration length is the real lever, not the provider.** Real audio-guide narrations are
   typically 2–4 minutes (~300–600 words) per stop; the curated corpus runs ~2–3x that. Trimming
   would cut cost roughly proportionally *and* likely fits the format better (nobody wants to stand
   in front of a statue for 10 minutes of audio). Free, fully in our control, worth testing before
   any vendor decision.
2. **The architecture already supports swapping TTS providers cheaply if ever needed** —
   `ports.TTSGenerator` is a real interface, `internal/adapters/elevenlabs/` is one implementation
   of it. A different/cheaper provider later means a new adapter, not a rewrite. Not an argument for
   switching now, just confirmation it's not a sunk decision.
3. A wholesale provider switch would likely mean giving up ElevenLabs' 1-2-minute voice cloning
   (what let the founder consider narrating in her own voice) — commodity TTS providers'
   voice-cloning offerings are typically heavier-weight. A hybrid (cloned voice via ElevenLabs,
   cheaper provider for premade-voice languages) is possible but adds real complexity — only worth
   it after confirming narration-length trimming alone isn't enough.

**Not decided yet**: whether to trim narrations, top up/upgrade the ElevenLabs plan, or both.
Whichever way this goes, it blocks generating the other 14 places, not anything already built.

## Environment note for next session

Integration tests in `internal/adapters/rabbitmq/*_test.go` connect directly to the real dev
RabbitMQ broker (`localhost:5672`, `tts_jobs` queue) — same broker the `docker-compose.yml` `worker`
container consumes from. Running both at once means two consumers racing on one queue: test
messages get delivered to the real worker and vice versa, producing confusing cross-contaminated
failures that look like a code bug but aren't (hit this today — cost real debugging time before the
cause was clear). **`docker compose stop worker` before running rabbitmq integration tests.**
