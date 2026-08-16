# Rio Audio Guide — Backend

Go backend for Rio Audio Guide, a multilingual geolocation audio guide. Manages places, narration
scripts, and generated audio files, and drives the text-to-speech generation pipeline over RabbitMQ.

## Architecture

Hexagonal architecture (ports & adapters) + Domain-Driven Design. Dependencies point inward:
adapters depend on ports, ports depend on domain, domain depends on nothing.

```
internal/
  domain/        Place, Script, AudioFile — entities, value objects, invariants. No framework code.
  ports/         Interfaces the domain/application layer depends on (repositories, publisher, storage).
  application/   Use cases orchestrating domain + ports (ReviewAndRequestAudio, StartAudioGeneration,
                 CompleteAudioGeneration).
  adapters/
    postgres/    PlaceRepository, ScriptRepository, AudioFileRepository — real PostgreSQL+PostGIS.
    rabbitmq/    AudioJobPublisher (driven) and Worker (driving) — two roles, same tts_jobs queue.
    s3/          AudioStorage — real AWS S3, no LocalStack/MinIO.
    http/        Echo HTTP server (GET /places, POST /scripts/:id/review).
cmd/
  api/           HTTP server binary — Postgres + RabbitMQ (publisher).
  worker/        TTS worker binary — Postgres + RabbitMQ (consumer) + S3. Separate from cmd/api so
                 the two can scale independently (HPA on the API, KEDA on the worker).
```

Three aggregates, not one: `Place`, `Script`, `AudioFile` are separate DDD aggregates (a place has
many scripts across languages; a script's audio generation lifecycle is distinct from the script
itself). See `docs/superpowers/specs/2026-08-12-backend-domain-model-design.md` for the full
reasoning.

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
- Docker (for local Postgres/RabbitMQ, or `kind` for local Kubernetes)
- A real AWS account with an S3 bucket (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` in your
  environment — never hardcoded, never committed)

## Running locally

Plain Docker containers for Postgres/RabbitMQ, `cmd/api`/`cmd/worker` run as normal local Go processes
(no Kubernetes involved at all — see "Running on `kind`" below if you specifically want to exercise the
Helm chart or autoscaling).

```bash
docker run -d --name rio-postgres -p 5433:5432 -e POSTGRES_PASSWORD=postgres postgis/postgis:16-3.4
docker run -d --name rio-rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management

psql "postgres://postgres:postgres@localhost:5433/postgres" -f internal/adapters/postgres/schema.sql

# One-time (or repeatable) import: Places/Scripts from the Python pipeline's CSVs into Postgres.
# See ../sourcing-pipeline/pipeline/curation/ for how those CSVs are produced.
DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
  go run ./cmd/import -places=<path>/places_clean_vN.csv -narrations=<path>/narrations_multi_full.csv

DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
RABBITMQ_URL="amqp://guest:guest@localhost:5672/" \
  go run ./cmd/api

DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
RABBITMQ_URL="amqp://guest:guest@localhost:5672/" \
S3_BUCKET="rio-audioguide-bucket" \
ELEVENLABS_API_KEY="<your key>" \
  go run ./cmd/worker
```

Trigger generation for an imported script (get a `voice_id` from the ElevenLabs voice library, or clone
one with `pipeline/curation/clone_voice.py`):

```bash
curl -X POST localhost:8080/scripts/<script-id>/review \
  -d '{"reviewer":"you","voice_id":"<voice_id>"}'
```

### Running on `kind` (local Kubernetes — to exercise the Helm chart/autoscaling)

Unlike the plain-Docker setup above, `kind` runs a real (if single-host) Kubernetes cluster, so
`deploy/helm/rio-backend` deploys as actual Pods/Deployments, and KEDA genuinely scales the worker on
`tts_jobs` queue depth (including scale-to-zero). It does not simulate real multi-node/cloud elasticity
(Karpenter node autoscaling needs a real cluster, e.g. EKS) — it's for validating the chart and
autoscaling logic for free before spending on real cloud infrastructure. See
`docs/superpowers/plans/2026-08-16-backend-mvp-completion.md`, Task 11, for the full walkthrough.

## Testing

```bash
go test ./...                                                    # unit tests, no external deps
go vet ./...
golangci-lint run ./...

TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" \
  go test -tags=integration ./...                                 # + real Postgres/RabbitMQ/S3
```

Integration tests are gated behind the `integration` build tag so `go test ./...` never needs live
infrastructure. The S3 integration test additionally requires `S3_TEST_BUCKET` and skips cleanly
without it.

## CI/CD

Three GitHub Actions workflows in `.github/workflows/`:

- **`backend-ci.yml`** — on every push/PR to `backend`. Three parallel jobs (`lint`, `test`,
  `security`/`govulncheck`), then `build`, then `integration-test` against real Postgres/RabbitMQ
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
