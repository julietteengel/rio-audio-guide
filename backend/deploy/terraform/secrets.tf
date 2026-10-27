# Secrets live in SSM Parameter Store (SecureString), never in user_data in
# plaintext and never in terraform.tfstate's own plaintext representation on
# disk where avoidable -- the EC2 instance reads them at boot via its IAM
# role (ssm.tf grants exactly ssm:GetParameter on these two names), not via
# values baked into the launch template.

resource "random_password" "db" {
  length  = 24
  special = false # avoid characters libpq connection strings need escaping
}

resource "random_password" "jwt_secret" {
  length  = 48
  special = false
}

resource "aws_ssm_parameter" "anthropic_api_key" {
  count = var.anthropic_api_key == null ? 0 : 1
  name  = "/rio-audio-guide/anthropic_api_key"
  type  = "SecureString"
  value = var.anthropic_api_key
}

resource "aws_ssm_parameter" "jwt_secret" {
  name  = "/rio-audio-guide/jwt_secret"
  type  = "SecureString"
  value = random_password.jwt_secret.result
}

resource "aws_ssm_parameter" "openai_api_key" {
  count = var.openai_api_key == null ? 0 : 1
  name  = "/rio-audio-guide/openai_api_key"
  type  = "SecureString"
  value = var.openai_api_key
}
