# Plain RDS Postgres (not Aurora) -- supports the postgis extension the
# schema needs (CREATE EXTENSION postgis in schema.sql) on any current
# engine version, no special engine flavor required.

resource "aws_security_group" "rds" {
  name        = "rio-audio-guide-rds"
  description = "Postgres reachable only from the app EC2 instance, never from the public internet."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description     = "Postgres from the app instance"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ec2.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_instance" "main" {
  identifier        = "rio-audio-guide"
  engine            = "postgres"
  instance_class    = var.db_instance_class
  allocated_storage = var.db_allocated_storage_gb

  db_name  = "postgres"
  username = "postgres"
  password = random_password.db.result

  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false
  skip_final_snapshot    = true # throwaway stack -- no final snapshot to clean up on destroy
  deletion_protection    = false

  # Below "Multi-AZ" / long backup retention on purpose: single-user
  # walk-test deployment, not a durability-critical production database.
  backup_retention_period = 1
  multi_az                = false
}
