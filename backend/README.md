# Rio Audio Guide — Backend

Go backend for Rio Audio Guide, a multilingual geolocation audio guide. Manages places, narration
scripts, and generated audio files, and drives the text-to-speech generation pipeline over RabbitMQ.

## Architecture

Hexagonal architecture (ports & adapters) + Domain-Driven Design. Dependencies point inward:
adapters depend on ports, ports depend on domain, domain depends on nothing.

```
internal/
  domain/        Place, Script, AudioFile — entities, value objects, invariants. No framework code.
  ports/         Interfaces the domain/application layer depends on (repositories, publisher, storage,
                 cache).
  application/   Use cases orchestrating domain + ports (ReviewAndRequestAudio, StartAudioGeneration,
                 CompleteAudioGeneration).
  adapters/
    postgres/    PlaceRepository, ScriptRepository, AudioFileRepository — real PostgreSQL+PostGIS.
    rabbitmq/    AudioJobPublisher (driven) and Worker (driving) — two roles, same tts_jobs queue.
    s3/          AudioStorage — real AWS S3, no LocalStack/MinIO. Uploads, and presigns GET URLs.
    redis/       Cache — cache-aside in front of the hot read routes. Fail-open: any Redis error is
                 treated as a miss, logged, and never fails the request.
    http/        Echo HTTP server (GET /places, GET /places/:id/audio, POST /scripts/:id/review).
cmd/
  api/           HTTP server binary — Postgres + RabbitMQ (publisher) + Redis + S3 (presigning only).
  worker/        TTS worker binary — Postgres + RabbitMQ (consumer) + S3. Separate from cmd/api so
                 the two can scale independently (HPA on the API, KEDA on the worker).
```

Three aggregates, not one: `Place`, `Script`, `AudioFile` are separate DDD aggregates (a place has
many scripts across languages; a script's audio generation lifecycle is distinct from the script
itself). See `../docs/superpowers/specs/2026-08-12-backend-domain-model-design.md` for the full
reasoning.

### What each layer actually does — traced through one request

The four `internal/` folders aren't just organizational — each one answers a different question, and
the dependency direction (`adapters → ports → application → domain`, never the reverse) means inner
layers never know the outer ones exist. Concretely, tracing `POST /scripts/{id}/review`:

1. **`adapters/http`** (`server.go`) parses the HTTP request (JSON body, path param) and calls into
   `application`. It knows about Echo, JSON, HTTP status codes — nothing below it does.
2. **`application`** (`ReviewAndRequestAudio` in `publish_script.go`) is the *use case*: the ordered
   steps a real request goes through, with no knowledge of Postgres, HTTP, or RabbitMQ specifics — only
   `domain` types and `ports` interfaces. Here: fetch the `Script` (via a port), tell it to transition
   to reviewed (a `domain` method), save it (via a port), create an `AudioFile` (`domain`), save it
   (via a port), publish a TTS job (via a port). This is the layer that would change if the *business
   process* changed (e.g. "require two reviewers") — not if the database changed.
3. **`ports`** (`ScriptRepository`, `AudioFileRepository`, `AudioJobPublisher` interfaces) is the
   *contract* `application` depends on — "something that can find/save a Script by ID," with zero
   detail about how. `application` is written entirely against these interfaces, never against a
   concrete Postgres type.
4. **`domain`** (`Script.MarkReviewed()`, `domain.NewAudioFile(...)`) enforces the actual business
   rules — e.g. `MarkReviewed()` refuses if the script isn't currently `draft`. This code has zero
   imports from this project outside `domain` itself — no SQL, no HTTP, no JSON tags.
5. **`adapters/postgres`**/**`adapters/rabbitmq`** are where the `ports` interfaces actually get
   implemented — real SQL, real AMQP calls. `application` never imports these packages directly; `main.go`
   is the only place that wires a concrete adapter into a port.

Why bother: `application`/`domain` can be unit-tested with fakes (no real Postgres/RabbitMQ needed —
see the `fake*Repo` types in the `_test.go` files), and swapping Postgres for something else would only
touch `adapters/postgres`, never the business logic.

TTS generation calls the real ElevenLabs API (`internal/adapters/elevenlabs/generator.go`, model
`eleven_multilingual_v2`) — the worker classifies failures as transient (429/5xx/408/timeout, requeued
with a 2s backoff and a capped retry count) or permanent (other 4xx — bad API key, unknown `voice_id`,
quota exceeded, rejected text — marks the `AudioFile` failed and stops retrying). Requires
`ELEVENLABS_API_KEY`. See `../docs/superpowers/specs/2026-08-19-elevenlabs-real-generation-findings.md`
for a real incident/fix history (an unbounded retry loop that silently re-billed ElevenLabs) worth
reading before touching this code.

`cmd/import` is a separate, manually-run CLI (not a long-lived service) that bridges the Python
sourcing/curation pipeline's output (`../pipeline/curation/places_export_v2.csv` +
`../pipeline/curation/narrations_multi_full.csv`) into Postgres as `Place`/`Script` rows, before any of
the above can run against real content.

## Requirements

- Go 1.25+
- Docker (for local Postgres/RabbitMQ/Redis, or `kind` for local Kubernetes)
- A real AWS account with an S3 bucket (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` in your
  environment — never hardcoded, never committed)

## Running locally

`docker compose up -d` (run from this `backend/` directory) starts everything — Postgres/RabbitMQ/Redis
plus `api`/`worker` themselves, built from `deploy/docker/Dockerfile.dev` (a `golang:1.26` image with
[air](https://github.com/air-verse/air) for hot reload). The repo root is bind-mounted into the
container (`.:/src`), so saving a `.go` file rebuilds and restarts that service in place — no
`docker compose build` round trip per change, no local Go toolchain required at all.

```bash
docker compose up -d              # postgres, postgres-test, rabbitmq, redis, api, worker
docker compose logs -f api        # follow one service's logs
docker compose up -d --build api  # only needed after editing Dockerfile.dev itself (not app code)

psql "postgres://postgres:postgres@localhost:5433/postgres" -f internal/adapters/postgres/schema.sql

# One-time (or repeatable) import: Places/Scripts from the Python pipeline's CSVs into Postgres.
DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
  go run ./cmd/import -places=../pipeline/curation/places_export_v2.csv \
    -narrations=../pipeline/curation/narrations_multi_full.csv
```

`api` needs nothing beyond what's already wired in `docker-compose.yml` — `DATABASE_URL`/
`RABBITMQ_URL` point at the `postgres`/`rabbitmq` service names (Compose's internal DNS), and
`REDIS_ADDR` is a bare `host:port` (`redis:6379`), not a URL — that's what the Redis client expects,
and `redis://…` would fail. Skipping Redis entirely still works: the cache is fail-open, so the API
serves every request correctly, just never from cache. It logs each failure rather than degrading
silently.

`worker` additionally needs a real `ELEVENLABS_API_KEY` (and, to actually upload finished audio, real
AWS credentials) — it refuses to start without one (`mustEnv`, deliberately, so it never silently
no-ops). Put these in a local, gitignored `.env` file next to `docker-compose.yml` (this `backend/`
directory); Compose reads it automatically:

```bash
# backend/.env — gitignored, never commit real values
ELEVENLABS_API_KEY=...
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
AWS_SESSION_TOKEN=...
AWS_REGION=us-east-1
```

Trigger generation for an imported script (get a `voice_id` from the ElevenLabs voice library, or clone
one with `../pipeline/curation/clone_voice.py`) — `reviewScript` sits behind `requireAuth` +
`requireRole(RoleAdmin)`, so it needs a real JWT from `POST /login`, not a body field:

```bash
curl -X POST localhost:8080/scripts/<script-id>/review \
  -H "Authorization: Bearer <token>" \
  -d '{"voice_id":"<voice_id>"}'
```

Then fetch the finished audio — note this takes a **place** ID, not a script ID, and the language
picks which of that place's scripts to serve:

```bash
curl "localhost:8080/places/<place-id>/audio?language=fr"
```

Three response shapes:

- `200 {"url":"https://rio-audio-guide.s3.amazonaws.com/…&X-Amz-Signature=…"}` — ready. The URL is
  presigned and expires after 15 minutes; `curl -o audio.mp3 "<url>"` downloads a playable MP3.
- `202 {"status":"…"}` — not ready. `queued`/`generating` means the worker hasn't finished, `failed`
  means it gave up (see the `AudioFile`'s `failure_reason`), and `script not yet published` means the
  audio exists but its script isn't published, so it isn't served.
- `404 {"error":"…"}` — no script for that place/language, or no audio was ever requested for it.

## Testing

```bash
go test ./...                                                        # unit tests, no external services
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5434/postgres" \
  go test -tags=integration ./...                                    # real Postgres/RabbitMQ/Redis/S3
```

Integration tests target the isolated `postgres-test` service (port 5434, no persistent volume, wiped
on `docker compose down -v`) — **not** the port-5433 one holding real dev data. If you're also running
`docker compose up -d worker` for real generation, stop it before running the RabbitMQ integration
suite: both would consume from the same real `tts_jobs` queue and produce confusing cross-contaminated
failures that look like a code bug but aren't.

## CI/CD

`../.github/workflows/backend-ci.yml` — lint/test/security/build/integration-test, triggers on any
push/PR touching `backend/**` on `master`. `../.github/workflows/docker-build.yml` builds and pushes
real images to ECR — **manual trigger only** (`workflow_dispatch`), deliberately not automatic while
there's no EKS cluster to deploy them to yet.
