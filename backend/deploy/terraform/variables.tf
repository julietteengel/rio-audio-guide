variable "aws_region" {
  description = "Region hosting this deployment -- same region as the existing rio-audio-guide S3 bucket."
  type        = string
  default     = "us-east-1"
}

variable "s3_bucket_name" {
  description = "Name of the already-existing S3 bucket for generated audio."
  type        = string
  default     = "rio-audio-guide"
}

variable "ec2_instance_type" {
  description = "Instance running api+worker+rabbitmq+redis as containers. t3.small: enough RAM to `go build` both binaries on first boot and run all four containers for one person's real-world testing, not meant for real multi-user load."
  type        = string
  default     = "t3.small"
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  type    = number
  default = 20
}

variable "anthropic_api_key" {
  description = "Anthropic API key for AI itineraries and the place assistant. Optional: leave null and those two routes fail while the rest of the API works. Pass via TF_VAR_anthropic_api_key, never commit it."
  type        = string
  sensitive   = true
  default     = null
}

variable "ses_sender_email" {
  description = "Verified SES sender address for account emails (registration/reset). Leave the default if you haven't verified a real address yet -- email sending will fail, but nothing else in the app depends on it for a quick walk-test."
  type        = string
  default     = "noreply@example.com"
}

variable "ssh_allowed_cidr" {
  description = "CIDR allowed to reach port 22 when ssh_key_name is set, e.g. \"203.0.113.7/32\" (your own IP). Never 0.0.0.0/0."
  type        = string
  default     = "127.0.0.1/32"

  validation {
    condition     = var.ssh_allowed_cidr != "0.0.0.0/0"
    error_message = "ssh_allowed_cidr must not be 0.0.0.0/0; use your own IP as a /32."
  }
}

variable "ssh_key_name" {
  description = "Optional: an existing EC2 key pair name, if you want SSH as a fallback alongside SSM Session Manager. Leave null to rely on SSM only (no inbound SSH port opened)."
  type        = string
  default     = null
}
