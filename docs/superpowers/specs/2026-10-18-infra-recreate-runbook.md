# 2026-10-18 — How to recreate the AWS deployment + ship a mobile build

A runbook, not a design doc: the exact, proven steps to get back to a working state after
everything was shut down tonight to stop AWS costs (see
`2026-10-18-live-testing-session-status.md` for why). Written from what actually worked this
session, not from the original (untested, incomplete) Terraform plan — see the caveats in each
section for where the two diverge.

**Database backup**: `~/rio-audio-guide-backups/rio-db-backup-20261018.pgdump` (on Juliette's
Mac, outside git, outside any session-temp folder). This is the single source of truth for all
real content (places, scripts, narration, audio references, itineraries) — nothing here is
reproducible by re-running the sourcing/curation pipeline from scratch.

## 1. Bring the AWS infrastructure back

Two cases, pick one.

### 1a. The EC2 instance still exists, just stopped

This is the normal case if nothing was destroyed beyond what tonight's cost cleanup covered
(instance `rio-audio-guide-test`, `i-05c15ee1ffa322d8c`, stopped — not terminated).

```bash
aws ec2 start-instances --region us-east-1 --instance-ids i-05c15ee1ffa322d8c
aws ec2 describe-instances --region us-east-1 --instance-ids i-05c15ee1ffa322d8c \
  --query "Reservations[0].Instances[0].PublicIpAddress" --output text
```

The instance gets a **new public IP** every time it starts (the old Elastic IP, 52.4.110.155,
was released tonight to stop paying for it). That means:

- A new `X-Y-Z-W.nip.io` HTTPS hostname (nip.io resolves any `X-Y-Z-W.nip.io` to that literal IP
  — no DNS setup needed, just build the string from the new IP).
- Caddy's config on the instance references the *old* hostname for its Let's Encrypt cert —
  update `/etc/caddy/Caddyfile` to the new one and `docker exec rio-backend-caddy-1 caddy reload
  --config /etc/caddy/Caddyfile` (overwrite the file with `printf ... > /etc/caddy/Caddyfile`,
  not `sed -i` — the file is a bind mount, `sed -i`'s rename-based edit fails on it with
  "Resource busy").
- EAS's `preview` environment variable `EXPO_PUBLIC_API_BASE_URL` needs the new URL (step 3) and
  a fresh build.
- If you want a **stable** IP that survives stop/start, allocate a new Elastic IP and associate
  it — but that's back to paying ~$3.6/month for it the whole time, which is exactly what got
  released tonight. Only do this if you're actively using the deployment again, not just testing
  once.

Once the instance is up and reachable by SSH/EC2 Instance Connect, the Docker containers
(`rio-audio-guide-api-1`, `-worker-1`, `-redis-1`, `-rabbitmq-1`, plus the separate
`rio-backend-*` stack that owns Caddy) should already be there from before — just
`cd ~/rio-audio-guide && sudo docker compose up -d` if they didn't auto-start on boot.

### 1b. The EC2 instance was terminated — rebuild from scratch

The clean path is Terraform, on the `infra/aws-rds-ec2-quick-deploy` branch (a separate worktree,
`.worktrees/infra-deploy` at the time of writing) — `backend/deploy/terraform/README.md` there has
the full `terraform init && terraform apply` / image-build / teardown steps.

**Important caveats this session found, not yet fixed in that plan:**

- **SSM never registered** on a freshly Terraform-created instance (tried twice, same symptom —
  root cause never confirmed). EC2 Instance Connect (browser-based SSH, Console → EC2 → Connect)
  worked as a fallback on the one instance that had a real SSH key pair; a brand-new
  Terraform-created instance may hit the same wall. Budget time for this, or create the instance
  manually (plain Ubuntu AMI, security group open on 22/80/443/8080) if Terraform's instance
  doesn't come up reachable within ~10 minutes.
- **No HTTPS in the Terraform plan at all** ("what this deliberately skips"). iOS's App
  Transport Security blocks plain `http://` outright — a standalone EAS build will not work
  against the Terraform stack's raw HTTP endpoint. After `terraform apply`, you still need to:
  1. Install Caddy on the instance.
  2. Point a `nip.io` hostname at the instance's IP (free, no DNS setup: `https://X-Y-Z-W.nip.io`
     for IP `X.Y.Z.W`).
  3. Write a Caddyfile reverse-proxying that hostname to the api container's port (8080) and
     reload Caddy — Let's Encrypt issues the cert automatically on first request.
- **RDS is `publicly_accessible = false`** by design (`rds.tf`) — reach it only through the EC2
  instance (SSM port-forward tunnel, see the terraform README's step 4, or `docker run` directly
  on the instance itself, which is what worked tonight).
- The Terraform `docker-compose.yml` template needs `ANTHROPIC_API_KEY` and `AWS_REGION` added
  to the `api` service's environment (not in the original plan) — AI itinerary generation and
  S3 presigning both fail silently/loudly without them respectively. Pull these from
  `backend/.env` locally.

## 2. Restore the real content into RDS

Once an RDS instance exists (from Terraform, or the one already running) and the EC2 instance can
reach it:

```bash
# From the EC2 instance, download the backup (scp it over, or re-upload to S3 and curl it down --
# whichever is more convenient at the time; it's ~1.7MB, trivial either way)

# Clear any stale mount artifact from a previous failed attempt first:
sudo rm -rf /tmp/rio-restore.pgdump

# Parse DATABASE_URL from docker-compose.yml instead of typing the password by hand --
# nano-pasting a long secret over SSH corrupted it more than once tonight:
DB_URL=$(grep -m1 DATABASE_URL ~/rio-audio-guide/docker-compose.yml | sed -E 's/.*DATABASE_URL:\s*"?([^"]+)"?.*/\1/')
export PGHOST=$(echo "$DB_URL" | sed -E 's#.*@([^:/]+).*#\1#')
export PGPORT=$(echo "$DB_URL" | sed -E 's#.*:([0-9]+)/.*#\1#')
export PGUSER=$(echo "$DB_URL" | sed -E 's#.*//([^:]+):.*#\1#')
export PGPASSWORD=$(echo "$DB_URL" | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')
export PGDATABASE=$(echo "$DB_URL" | sed -E 's#.*/([^/?]+)(\?.*)?$#\1#')

docker run --rm \
  -v /tmp/rio-restore.pgdump:/dump.pgdump \
  -e PGPASSWORD="$PGPASSWORD" \
  postgres:16 \
  pg_restore --clean --if-exists -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" /dump.pgdump
```

Use `postgres:16` for both the original dump and this restore — a local `pg_restore`/`pg_dump`
version mismatch against the server's major version fails outright.

## 3. Point the mobile app at the backend and ship a build

`EXPO_PUBLIC_API_BASE_URL` for a standalone EAS build comes from **EAS's own server-side
environment variable**, not the local `mobile/.env` — check/set it with:

```bash
cd mobile
eas env:list --environment preview
eas env:create --environment preview --name EXPO_PUBLIC_API_BASE_URL --value "https://NEW-IP-WITH-DASHES.nip.io" --visibility plaintext
```

Then build and get an install link:

```bash
npx eas-cli build --profile preview --platform ios --non-interactive
```

The command prints an `expo.dev/.../builds/<id>` link at the end — open that on the iPhone to
install. There is **no OTA/live-reload** for this build profile (no `expo-updates` configured):
every code change needs a brand-new build + reinstall via a fresh link, not just a reload.

## 4. Known pitfalls to not repeat

- **Never paste a long secret into `nano` over SSH.** It corrupted the Anthropic API key's value
  (extra characters appended) more than once tonight despite looking correct on screen. Use a
  `read -r` prompt + `sed -i` one-liner instead (shown in the DB-restore section's pattern, same
  idea) — paste once into the shell prompt, not into an open editor buffer.
- **`curl` to `api.anthropic.com` may fail with `HTTP/2 stream ... PROTOCOL_ERROR`** on this
  Ubuntu instance specifically — add `--http1.1` to any manual `curl` test against Anthropic's
  API from that host. This is a curl/TLS quirk on that machine, not necessarily something the
  Go backend itself (different HTTP/2 implementation) is affected by.
- **Two Docker Compose stacks can coexist on the same instance** (`rio-audio-guide-*` and an
  older `rio-backend-*`, the latter owning Caddy+its own Postgres) — `sudo docker ps -a` to check
  before assuming there's only one `api` container. Caddy on this instance reaches the
  `rio-audio-guide` stack's `api` container via the Docker bridge gateway IP (`172.17.0.1:8080`,
  the host-published port), not a shared Docker network name, since the two stacks aren't on the
  same network.
- **Building Docker images on an Apple Silicon Mac for this (x86_64) EC2 instance** needs
  `docker buildx build --platform linux/amd64 ...` — a plain `docker build` produces an image the
  instance's Docker rejects with "no matching manifest."
