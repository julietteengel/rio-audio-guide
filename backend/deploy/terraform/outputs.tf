output "api_url" {
  description = "Point EXPO_PUBLIC_API_BASE_URL (mobile/.env) at this."
  value       = "http://${aws_instance.app.public_ip}:8080"
}

output "ec2_instance_id" {
  description = "For SSM Session Manager: aws ssm start-session --target <this>"
  value       = aws_instance.app.id
}

output "rds_endpoint" {
  value = aws_db_instance.main.address
}

output "ecr_api_repo_url" {
  value = aws_ecr_repository.api.repository_url
}

output "ecr_worker_repo_url" {
  value = aws_ecr_repository.worker.repository_url
}

output "db_password" {
  value     = random_password.db.result
  sensitive = true
}
