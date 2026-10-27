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

variable "elevenlabs_api_key" {
  description = "Real ElevenLabs key -- the worker refuses to start without one. Pass via -var or TF_VAR_elevenlabs_api_key, never commit it."
  type        = string
  sensitive   = true
}

variable "ses_sender_email" {
  description = "Verified SES sender address for account emails (registration/reset). Leave the default if you haven't verified a real address yet -- email sending will fail, but nothing else in the app depends on it for a quick walk-test."
  type        = string
  default     = "noreply@example.com"
}

variable "ssh_key_name" {
  description = "Optional: an existing EC2 key pair name, if you want SSH as a fallback alongside SSM Session Manager. Leave null to rely on SSM only (no inbound SSH port opened)."
  type        = string
  default     = null
}
