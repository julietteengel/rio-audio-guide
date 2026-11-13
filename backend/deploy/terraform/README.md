# Quick real-AWS deployment (RDS + one EC2 instance)

Not the EKS/Helm setup under `deploy/k8s`/`deploy/helm` -- this is the fast,
reliable path when you need a real AWS-backed backend quickly: one RDS
Postgres instance, one EC2 instance running `api` + `worker` + `rabbitmq` +
`redis` as containers via `docker-compose`.

## Prerequisites

- Working AWS credentials (`aws sts get-caller-identity` succeeds).
- Optionally an OpenAI API key (the worker uses OpenAI text-to-speech and Whisper when it is set, AWS Polly otherwise).
- Optionally an Anthropic API key (AI itineraries and the place assistant need it; the rest of the API works without).
- Docker running locally (to build and push the two images).

## 1. Apply the infrastructure

```bash
cd backend/deploy/terraform
terraform init
export TF_VAR_anthropic_api_key=<your key>   # optional
export TF_VAR_openai_api_key=<your key>      # optional
terraform apply
```

This creates: 2 ECR repos, an RDS instance, an EC2 instance, IAM
role/policies, security groups, and 3 SSM SecureString parameters (the JWT
signing secret and the Anthropic and OpenAI keys). It does **not** create the S3 bucket (`rio-audio-guide`
already exists) or a VPC (reuses the account's default VPC/subnets).

RDS creation alone takes 5-10 minutes; the EC2 instance registers with SSM
(needed for step 4 and for debugging) a minute or two after it boots, not
instantly -- `aws ssm describe-instance-information` returning nothing yet
for a brand new instance is normal, not a failure.

The EC2 instance's `user_data` applies `schema.sql` to the fresh RDS
instance automatically on first boot -- no separate migration step. Check
`/var/log/rio-audio-guide-userdata.log` on the instance (via SSM) if it
doesn't come up.

## 2. Build and push the two images

The instance's `user_data` pulls `:latest` from each ECR repo -- nothing
runs until you've pushed at least one tag. From `backend/`:

```bash
REGION=us-east-1
API_REPO=$(terraform -chdir=deploy/terraform output -raw ecr_api_repo_url)
WORKER_REPO=$(terraform -chdir=deploy/terraform output -raw ecr_worker_repo_url)

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "${API_REPO%%/*}"

docker build -f deploy/docker/Dockerfile.api -t "$API_REPO:latest" .
docker push "$API_REPO:latest"

docker build -f deploy/docker/Dockerfile.worker -t "$WORKER_REPO:latest" .
docker push "$WORKER_REPO:latest"
```

Then either wait for the next instance boot, or SSH/SSM in and run
`cd /opt/rio-audio-guide && docker-compose pull && docker-compose up -d`
directly.

## 3. Point the mobile app at it

```bash
terraform -chdir=deploy/terraform output -raw api_url
```

Put that in `mobile/.env` as `EXPO_PUBLIC_API_BASE_URL`.

## 4. Carry over the real content already generated locally

This stack only creates an empty, schema'd database -- `cmd/import` alone
would only reload the raw sourcing CSVs, missing every place's real
narration, translations, audio, and images already generated and sitting
in the local dev Postgres (not reproducible from a re-run). Dump and
restore instead.

RDS is deliberately `publicly_accessible = false` (only the EC2 instance's
security group can reach port 5432, see `rds.tf`) -- don't open that up
just for convenience. Reach it from a laptop through an SSM port-forward
tunnel via the EC2 instance instead, no public exposure needed:

```bash
# Dump: your local pg_dump must match the server's major version (16) --
# Homebrew's is often older, so dump from inside the matching postgres
# container rather than failing on a version mismatch.
docker exec backend-postgres-1 pg_dump -U postgres -d postgres \
  --no-owner --no-privileges -F c -f /tmp/rio-dump.pgdump
docker cp backend-postgres-1:/tmp/rio-dump.pgdump /tmp/rio-local-dump.pgdump

# Tunnel (run in the background -- it's a persistent session):
aws ssm start-session --region us-east-1 \
  --target "$(terraform -chdir=deploy/terraform output -raw ec2_instance_id)" \
  --document-name AWS-StartPortForwardingSessionToRemoteHost \
  --parameters '{"host":["'"$(terraform -chdir=deploy/terraform output -raw rds_endpoint)"'"],"portNumber":["5432"],"localPortNumber":["15432"]}'

# Restore, through the tunnel's local port -- same version-matching reason
# as the dump step, so via a postgres:16 container, not local pg_restore:
docker run --rm -v /tmp/rio-local-dump.pgdump:/dump.pgdump \
  -e PGPASSWORD="$(terraform -chdir=deploy/terraform output -raw db_password)" \
  --network host postgres:16-alpine \
  pg_restore -h localhost -p 15432 -U postgres -d postgres \
  --no-owner --no-privileges /dump.pgdump
```

Only needed once, or again after a local session that adds more real
content worth carrying forward.

## Debugging without SSH

```bash
aws ssm start-session --target "$(terraform -chdir=deploy/terraform output -raw ec2_instance_id)"
```

## Tearing down

```bash
terraform destroy
```

Deletes everything this stack created (RDS, EC2, ECR repos, IAM role, SSM
parameters). The S3 bucket is untouched (it was only ever referenced, never
owned by this stack).

## What this deliberately skips

No HTTPS/TLS, no custom domain, no multi-AZ, no autoscaling, no CI pipeline
for image builds, no VPC of its own. All reasonable upgrades later, none of
them required to get a real, working, AWS-backed backend up quickly.
