# Quick real-AWS deployment (RDS + one EC2 instance)

Not the EKS/Helm setup under `deploy/k8s`/`deploy/helm` -- this is the fast,
reliable path when you need a real AWS-backed backend quickly: one RDS
Postgres instance, one EC2 instance running `api` + `worker` + `rabbitmq` +
`redis` as containers via `docker-compose`.

## Prerequisites

- Working AWS credentials (`aws sts get-caller-identity` succeeds).
- A real ElevenLabs API key.
- Docker running locally (to build and push the two images).

## 1. Apply the infrastructure

```bash
cd backend/deploy/terraform
terraform init
terraform apply -var="elevenlabs_api_key=<your real key>"
```

This creates: 2 ECR repos, an RDS instance, an EC2 instance, IAM
role/policies, security groups, and 2 SSM SecureString parameters. It does
**not** create the S3 bucket (`rio-audio-guide` already exists) or a VPC
(reuses the account's default VPC/subnets).

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

## 4. Run the narration batch review against this real database

`cmd/batchreview` (see `backend/cmd/batchreview`) needs `DATABASE_URL`
pointed at the real RDS instance, not local Postgres:

```bash
export DATABASE_URL="postgres://postgres:$(terraform -chdir=deploy/terraform output -raw db_password)@$(terraform -chdir=deploy/terraform output -raw rds_endpoint):5432/postgres"
```

It still needs `cmd/import` run first against this same `DATABASE_URL` to
actually load places/scripts -- this stack only creates an empty, schema'd
database.

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
