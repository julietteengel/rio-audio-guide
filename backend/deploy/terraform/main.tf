# Minimal, fast-to-stand-up deployment: RDS Postgres + one EC2 instance
# running the api/worker/rabbitmq/redis containers via docker compose -- NOT
# the EKS/Helm setup in deploy/k8s and deploy/helm (that one needs its own,
# separately-planned effort; this is the "need a real AWS-backed deployment
# quickly, reliably" path). Reuses the account's default VPC/subnets and the
# already-existing `rio-audio-guide` S3 bucket rather than recreating them.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# Already exists (not created/destroyed by this stack) -- the worker uploads
# generated audio here and the API presigns GET URLs against it.
data "aws_s3_bucket" "audio" {
  bucket = var.s3_bucket_name
}
