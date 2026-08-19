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
itself). See `docs/superpowers/specs/2026-08-12-backend-domain-model-design.md` for the full
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
`eleven_multilingual_v2`) — the worker classifies failures as transient (429/5xx/408, requeued with a
2s delay) or permanent (other 4xx — bad API key, unknown `voice_id`, rejected text — marks the
`AudioFile` failed and stops retrying). Requires `ELEVENLABS_API_KEY`.

`cmd/import` is a separate, manually-run CLI (not a long-lived service) that bridges the Python
sourcing/curation pipeline's output (`pipeline/curation/places_clean_vN.csv` +
`pipeline/curation/narrations_multi_full.csv`, produced on the `sourcing-pipeline` branch) into
Postgres as `Place`/`Script` rows, before any of the above can run against real content.

## Requirements

- Go 1.25+
- Docker (for local Postgres/RabbitMQ/Redis, or `kind` for local Kubernetes)
- A real AWS account with an S3 bucket (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` in your
  environment — never hardcoded, never committed)

## Running locally

Plain Docker containers for Postgres/RabbitMQ/Redis, `cmd/api`/`cmd/worker` run as normal local Go
processes (no Kubernetes involved at all — see "Running on `kind`" below if you specifically want
to exercise the Helm chart or autoscaling).

```bash
docker run -d --name rio-postgres -p 5433:5432 -e POSTGRES_PASSWORD=postgres postgis/postgis:16-3.4
docker run -d --name rio-rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
docker run -d --name rio-redis -p 6379:6379 redis:7-alpine

psql "postgres://postgres:postgres@localhost:5433/postgres" -f internal/adapters/postgres/schema.sql

# One-time (or repeatable) import: Places/Scripts from the Python pipeline's CSVs into Postgres.
# See ../sourcing-pipeline/pipeline/curation/ for how those CSVs are produced.
DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
  go run ./cmd/import -places=<path>/places_clean_vN.csv -narrations=<path>/narrations_multi_full.csv

DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
RABBITMQ_URL="amqp://guest:guest@localhost:5672/" \
REDIS_ADDR="localhost:6379" \
  go run ./cmd/api

DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
RABBITMQ_URL="amqp://guest:guest@localhost:5672/" \
S3_BUCKET="rio-audio-guide" \
ELEVENLABS_API_KEY="<your key>" \
  go run ./cmd/worker
```

`REDIS_ADDR` is a bare `host:port`, not a URL like `DATABASE_URL`/`RABBITMQ_URL` — that's what the
Redis client expects, and `redis://…` would fail. Skipping Redis entirely still works: the cache is
fail-open, so the API serves every request correctly, just never from cache. It logs each failure
rather than degrading silently.

Trigger generation for an imported script (get a `voice_id` from the ElevenLabs voice library, or clone
one with `pipeline/curation/clone_voice.py`):

```bash
curl -X POST localhost:8080/scripts/<script-id>/review \
  -d '{"reviewer":"you","voice_id":"<voice_id>"}'
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
  Distinct from `500`, which means a real backend failure, not "doesn't exist".

### Running on `kind` (local Kubernetes — to exercise the Helm chart/autoscaling)

Unlike the plain-Docker setup above, `kind` runs a real (if single-host) Kubernetes cluster, so
`deploy/helm/rio-backend` deploys as actual Pods/Deployments, and KEDA genuinely scales the worker on
`tts_jobs` queue depth (including scale-to-zero). It does not simulate real multi-node/cloud elasticity
(Karpenter node autoscaling needs a real cluster, e.g. EKS) — it's for validating the chart and
autoscaling logic for free before spending on real cloud infrastructure. See
`docs/superpowers/plans/2026-08-16-backend-mvp-completion.md`, Task 11, for the full walkthrough.

Postgres, RabbitMQ and Redis are installed as *separate* releases, not as `Chart.yaml` dependencies —
`deploy/helm/rio-backend` only deploys the API and the worker, and points at whatever is already
running under those service names:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami && helm repo update
helm install demo-postgres bitnami/postgresql --set auth.postgresPassword=postgres --set auth.database=postgres
helm install demo-redis bitnami/redis --set auth.enabled=false
```

`auth.enabled=false` on Redis is deliberate, not laziness: the cache client (`cmd/api/main.go`) takes
only `REDIS_ADDR`, with no password env var — a password-protected Redis would fail every command,
and the cache's fail-open design would swallow that silently forever. Nothing secret transits the
cache anyway (public place listings and presigned URLs).

RabbitMQ has no Bitnami line here on purpose: that chart hits the Bitnami license wall
(`ImagePullBackOff`), so it's deployed as a minimal `Deployment`+`Service` named `demo-rabbitmq` —
see Task 11 Step 3 of the plan above for the manifest. The Postgres and Redis charts are both still
free to pull.

Service names matter: `values.yaml` defaults to `demo-redis-master:6379` (the Bitnami Redis chart's
master `Service`), so the release must be named `demo-redis` for that default to resolve — otherwise
override `redis.addr`.

## Testing

```bash
go test ./...                                                    # no infra needed (2 hit the network)
go vet ./...
golangci-lint run ./...

TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5434/postgres" \
  go test -tags=integration ./...                                 # + real Postgres/RabbitMQ/Redis/S3
```

`TEST_DATABASE_URL` points at a *separate* Postgres (port 5434, the `postgres-test` service in
`docker-compose.yml`), not the port-5433 one holding real dev data. Integration tests write real
fixture rows (`"Test Place"`, `"julie"` as a reviewer, etc.) — pointing them at the dev database
silently pollutes it with that content over repeated runs.

Everything needing a live Postgres/RabbitMQ/Redis is gated behind the `integration` build tag, so
`go test ./...` needs no infrastructure to be running. Two untagged tests do touch the network, but
neither needs anything provisioned — both are failure-path tests, and the failure is the assertion:

- the S3 permanent-error test calls real AWS with deliberately invalid credentials and asserts the
  `InvalidAccessKeyId` response is classified as permanent (no bucket, no valid credentials needed —
  but it does need outbound network, and will fail offline);
- the Redis connection-error test opens a socket to a port nothing listens on and asserts the failure
  is reported as an error rather than silently as a cache miss (no Redis needed).

`TestAudioStorage_Upload`, which does need a real writable bucket, requires `S3_TEST_BUCKET` and
skips cleanly without it.

The `integration` run additionally needs Redis on `localhost:6379` (override with `TEST_REDIS_ADDR`)
alongside Postgres and RabbitMQ — CI provides all three as service containers.

## CI/CD

Three GitHub Actions workflows in `.github/workflows/`:

- **`backend-ci.yml`** — on every push/PR to `backend`. Three parallel jobs (`lint`, `test`,
  `security`/`govulncheck`), then `build`, then `integration-test` against real Postgres/RabbitMQ/Redis
  service containers.
- **`docker-build.yml`** — builds and pushes `rio-api`/`rio-worker` images to ECR when `cmd/**`,
  `internal/**`, or a Dockerfile changes.
- **`k8s-deploy.yml`** — deploys to EKS via Helm once `docker-build.yml` succeeds.

## Deployment

- **`deploy/docker/`** — multi-stage Dockerfiles (distroless runtime images) for both binaries.
- **`deploy/helm/rio-backend/`** — Helm chart: API (`Deployment`/`Service`/`HorizontalPodAutoscaler`
  on CPU) and worker (`Deployment`/KEDA `ScaledObject` on `tts_jobs` queue depth, scales to zero when
  idle).
- **`deploy/k8s/canary-istio/`** and **`deploy/k8s/blue-green/`** — two alternative, independently
  documented rollout strategies for shipping a new API version (percentage-based traffic split via
  Istio vs. atomic `Service`-selector switch). Not combined — pick one per rollout.
- **`deploy/k8s/karpenter-nodepool-example.yaml`** — illustrative node-autoscaling config for a real
  EKS cluster with Karpenter installed.

Validated end-to-end on a local `kind` cluster (Docker Desktop, no cloud cost): both images build,
the Helm chart deploys cleanly, Postgres/RabbitMQ run in-cluster, the API serves real HTTP requests
against real Postgres, and KEDA genuinely scales the worker (including scale-to-zero when the queue
is empty). A real EKS deployment uses the same chart and Dockerfiles — see
`docs/superpowers/plans/2026-08-16-backend-mvp-completion.md`, Task 11, for the full walkthrough
including two real bugs found and fixed by testing against an actual cluster.

## Design docs

Point-in-time decisions and specs live in `docs/superpowers/specs/`; per-feature implementation plans
(task-by-task, TDD) live in `docs/superpowers/plans/`. Both are append-only history, not living docs —
check `git log` on a given file for the latest revision rather than assuming it's current.
