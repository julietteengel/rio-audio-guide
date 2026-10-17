resource "aws_security_group" "ec2" {
  name        = "rio-audio-guide-app"
  description = "The app instance: API port public (mobile app needs to reach it), RabbitMQ/Redis stay container-internal, SSH closed (SSM Session Manager instead unless ssh_key_name is set)."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "Backend API, reachable from the mobile app over the internet"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  dynamic "ingress" {
    for_each = var.ssh_key_name == null ? [] : [1]
    content {
      description = "SSH fallback -- only present when ssh_key_name is set"
      from_port   = 22
      to_port     = 22
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "app" {
  name               = "rio-audio-guide-app"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

# SSM Session Manager access without opening an inbound SSH port.
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Pull images from the two repos this stack owns, nothing broader.
resource "aws_iam_role_policy_attachment" "ecr_pull" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

data "aws_iam_policy_document" "app" {
  statement {
    sid       = "AudioBucketReadWrite"
    actions   = ["s3:GetObject", "s3:PutObject"]
    resources = ["${data.aws_s3_bucket.audio.arn}/*"]
  }
  statement {
    sid       = "PollySynthesize"
    actions   = ["polly:SynthesizeSpeech"]
    resources = ["*"] # Polly has no per-voice/per-request resource-level permissions to scope to
  }
  statement {
    sid     = "ReadOwnSecrets"
    actions = ["ssm:GetParameter"]
    resources = [
      aws_ssm_parameter.elevenlabs_api_key.arn,
      aws_ssm_parameter.jwt_secret.arn,
    ]
  }
  statement {
    sid       = "SendAccountEmails"
    actions   = ["ses:SendEmail", "ses:SendRawEmail"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "app" {
  name   = "rio-audio-guide-app"
  role   = aws_iam_role.app.id
  policy = data.aws_iam_policy_document.app.json
}

resource "aws_iam_instance_profile" "app" {
  name = "rio-audio-guide-app"
  role = aws_iam_role.app.name
}

resource "aws_instance" "app" {
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.ec2_instance_type
  subnet_id              = data.aws_subnets.default.ids[0]
  vpc_security_group_ids = [aws_security_group.ec2.id]
  iam_instance_profile   = aws_iam_instance_profile.app.name
  key_name               = var.ssh_key_name

  # Depends on the DB existing (endpoint is baked into user_data) and on the
  # ECR repos existing (user_data pulls from them) -- Terraform infers both
  # from the references below, listed here only for human clarity.
  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    db_endpoint         = aws_db_instance.main.address
    db_password         = random_password.db.result
    s3_bucket           = data.aws_s3_bucket.audio.id
    aws_region          = var.aws_region
    ecr_api_repo_url    = aws_ecr_repository.api.repository_url
    ecr_worker_repo_url = aws_ecr_repository.worker.repository_url
    ecr_registry        = split("/", aws_ecr_repository.api.repository_url)[0]
    ses_sender_email    = var.ses_sender_email
    elevenlabs_param    = aws_ssm_parameter.elevenlabs_api_key.name
    jwt_secret_param    = aws_ssm_parameter.jwt_secret.name
    schema_sql          = file("${path.module}/../../internal/adapters/postgres/schema.sql")
  })
  user_data_replace_on_change = true

  tags = {
    Name = "rio-audio-guide-app"
  }
}
